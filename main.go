package main

import (
	"bytes"
	"fmt"
	"go/build"
	"go/format"
	"go/types"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/tools/go/loader"
)

type Methoder interface {
	Method(i int) *types.Func
	NumMethods() int
}

func main() {
	log.SetFlags(0)
	log.SetPrefix(filepath.Base(os.Args[0]) + ": ")

	//pkgName := "database/sql"
	//typeName := "DB"
	pkgName := "./testdata/pkg"
	typeName := "T"
	var buf bytes.Buffer
	if err := codegen(&buf, pkgName, typeName); err != nil {
		log.Fatalln(err)
	}
	code, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Printf("%s", code)
}

var codeTemplate = strings.TrimSpace(`
package {{.Package}}

{{$type := printf "err%s" .Name}}
type {{$type}} struct {
	Val *{{.Name}}
	Err error
}

{{range .Methods}}
func (p *{{$type}}) {{.Name}}{{.Params}} {
	if p.Err != nil {
		return
	}
	{{with .Returns}}{{.}} = {{end}}p.Val.{{.Name}}({{.Args}})
}
{{end}}
`)

type methodParam struct {
	Name    string
	Params  *types.Tuple
	Results *types.Tuple
}

func (m *methodParam) Args() string {
	args := make([]string, m.Params.Len())
	for i := 0; i < m.Params.Len(); i++ {
		v := m.Params.At(i)
		if v.Name() == "" {
			args[i] = zeroValue(v.Type())
		} else {
			args[i] = v.Name()
		}
	}
	return strings.Join(args, ", ")
}

func (m *methodParam) Returns() string {
	n := m.Results.Len()
	vars := make([]string, n)
	if m.isTrailingErr() {
		n--
	}
	for i := 0; i < n; i++ {
		vars[i] = "_"
	}
	if m.isTrailingErr() {
		vars[n] = "p.Err"
	}
	return strings.Join(vars, ", ")
}

func (m *methodParam) lastResult() types.Type {
	if m.Results == nil || m.Results.Len() == 0 {
		return nil
	}
	return m.Results.At(m.Results.Len() - 1).Type()
}

func (m *methodParam) isTrailingErr() bool {
	last := m.lastResult()
	return isErrorType(last)
}

type typeParam struct {
	Package string
	Name    string
	Methods []*methodParam
}

func codegen(w io.Writer, pkgName, typeName string) error {
	tmpl := template.Must(template.New("code").Parse(codeTemplate))

	ctx := build.Default
	//ctx.BuildTags = []string{}
	pkg, err := ctx.Import(".", ".", 0)
	if err != nil {
		return err
	}
	var param typeParam
	param.Package = pkg.Name
	param.Name = typeName
	meths, err := exportedMethods(pkgName, typeName)
	if err != nil {
		return err
	}
	for _, m := range meths {
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return fmt.Errorf("%v: don't have signature?", m)
		}
		param.Methods = append(param.Methods, &methodParam{
			Name:    m.Name(),
			Params:  sig.Params(),
			Results: sig.Results(),
		})
	}
	return tmpl.Execute(w, &param)
}

func zeroValue(t types.Type) string {
	switch v := t.(type) {
	case *types.Basic:
		switch v.Kind() {
		case types.Bool:
			return "false"
		case types.String:
			return `""`
		default:
			return "0"
		}
	case *types.Named:
		return fmt.Sprintf("%s{}", v.Obj().Id())
	default:
		return "nil"
	}
}

func isErrorType(t types.Type) bool {
	v, ok := t.(*types.Named)
	if !ok {
		return false
	}
	typeName := v.Obj()
	return typeName.Name() == "error" && typeName.Pkg() == nil
}

func parsePkg(pkg string) (*types.Package, error) {
	var conf loader.Config
	conf.Import(pkg)
	prog, err := conf.Load()
	if err != nil {
		return nil, err
	}
	return prog.Package(pkg).Pkg, nil
}

func exportedMethods(pkgName, typeName string) ([]*types.Func, error) {
	pkg, err := parsePkg(pkgName)
	if err != nil {
		return nil, err
	}
	p := pkg.Scope().Lookup(typeName)
	if p == nil {
		return nil, fmt.Errorf("%s.%s is not exist", pkgName, typeName)
	}
	if _, ok := p.(*types.TypeName); !ok {
		return nil, fmt.Errorf("%s.%s is not a named type", pkgName, typeName)
	}
	m, ok := p.Type().(Methoder)
	if !ok {
		return nil, nil
	}
	var meths []*types.Func
	for i := 0; i < m.NumMethods(); i++ {
		f := m.Method(i)
		if !f.Exported() {
			continue
		}
		meths = append(meths, f)
	}
	return meths, nil
}
