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

{{if .Imports}}
import (
	{{- range .Imports}}
	{{. | printf "%q"}}
	{{- end}}
)
{{end}}

{{$name := printf "err%s" .Name}}
type {{$name}} struct {
	Val *{{.Type}}
	Err error
}

{{range .Methods}}
func (p *{{$name}}) {{.Name}}({{.Params}}) {
	if p.Err != nil {
		return
	}
	{{with .Returns}}{{.}} = {{end}}p.Val.{{.Name}}({{.Args}})
}
{{end}}
`)

type methodParam struct {
	Name    string
	params  *types.Tuple
	Results *types.Tuple
}

func NewMethod(name string, params, results *types.Tuple) *methodParam {
	return &methodParam{
		Name:    name,
		params:  params,
		Results: results,
	}
}

func (m *methodParam) Imports() []string {
	var a []string
	for i := 0; i < m.params.Len(); i++ {
		v := m.params.At(i)
		path, _ := importPath(v.Type())
		if path == "" {
			continue
		}
		a = append(a, path)
	}
	return Uniq(a)
}

func (m *methodParam) Params() string {
	args := make([]string, m.params.Len())
	for i := 0; i < m.params.Len(); i++ {
		v := m.params.At(i)
		typeName := canonicalType(v.Type())
		args[i] = fmt.Sprintf("%s %s", v.Name(), typeName)
	}
	return strings.Join(args, ", ")
}

func importPath(t types.Type) (path, name string) {
	p, ok := t.(*types.Named)
	if !ok {
		return "", t.String()
	}
	n := p.Obj()
	if pkg := n.Pkg(); pkg != nil {
		return pkg.Path(), fmt.Sprintf("%s.%s", pkg.Name(), n.Name())
	}
	return "", n.Name()
}

func canonicalType(t types.Type) string {
	_, name := importPath(t)
	return name
}

func (m *methodParam) Args() string {
	args := make([]string, m.params.Len())
	for i := 0; i < m.params.Len(); i++ {
		v := m.params.At(i)
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
	Type    string
	Methods []*methodParam

	importPaths []string
}

func Uniq(a []string) []string {
	m := make(map[string]struct{})
	for _, s := range a {
		m[s] = struct{}{}
	}
	t := make([]string, 0, len(m))
	for s := range m {
		t = append(t, s)
	}
	return t
}

func (t *typeParam) Imports() []string {
	var a []string
	for _, m := range t.Methods {
		a = append(a, m.Imports()...)
	}
	return Uniq(a)
}

func codegen(w io.Writer, pkgPath, typeName string) error {
	tmpl := template.Must(template.New("code").Parse(codeTemplate))

	ctx := build.Default
	//ctx.BuildTags = []string{}
	currentPkg, err := ctx.Import(".", ".", 0)
	if err != nil {
		return err
	}
	var param typeParam
	param.Package = currentPkg.Name

	pkg, err := parsePkg(pkgPath)
	if err != nil {
		return err
	}
	param.Name = typeName
	param.Type = fmt.Sprintf("%s.%s", pkg.Name(), typeName)

	meths, err := exportedMethods(pkg, typeName)
	if err != nil {
		return err
	}
	for _, m := range meths {
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return fmt.Errorf("%v: don't have signature?", m)
		}
		p := NewMethod(m.Name(), sig.Params(), sig.Results())
		param.Methods = append(param.Methods, p)
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
		return fmt.Sprintf("%s{}", canonicalType(t))
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

func exportedMethods(pkg *types.Package, name string) ([]*types.Func, error) {
	p := pkg.Scope().Lookup(name)
	if p == nil {
		return nil, fmt.Errorf("%s.%s is not exist", pkg.Name(), name)
	}
	if _, ok := p.(*types.TypeName); !ok {
		return nil, fmt.Errorf("%s.%s is not a named type", pkg.Name(), name)
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
