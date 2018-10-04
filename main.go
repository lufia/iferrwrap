package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
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
		args[i] = m.Params.At(i).Name()
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
	pkg, err := ctx.Import(pkgName, ".", 0)
	if err != nil {
		return err
	}
	t, err := lookupType(pkg.Dir, pkg.Name, typeName)
	if err != nil {
		return err
	}
	var param typeParam
	param.Package = "xxx" // current package name
	param.Name = typeName
	meths, err := t.Methods()
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

func isErrorType(t types.Type) bool {
	v, ok := t.(*types.Named)
	if !ok {
		return false
	}
	typeName := v.Obj()
	return typeName.Name() == "error" && typeName.Pkg() == nil
}

func parsePkg(dir, name string) (*types.Package, *types.Info, error) {
	fs := token.NewFileSet()
	// TODO(lufia): ParseDir(...filter(_test.go))
	pkgs, err := parser.ParseDir(fs, dir, nil, 0)
	if err != nil {
		return nil, nil, err
	}
	pkg, ok := pkgs[name]
	if !ok {
		return nil, nil, fmt.Errorf("package %s is not exist", name)
	}

	cfg := types.Config{
		Importer: importer.For("source", nil),
	}
	files := make([]*ast.File, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		files = append(files, f)
	}

	info := &types.Info{
		Defs: make(map[*ast.Ident]types.Object),
	}
	p, err := cfg.Check(dir, fs, files, info)
	if err != nil {
		return nil, nil, err
	}
	return p, info, nil
}

type Type struct {
	pkg *types.Package
	typ types.Type
}

func lookupType(dir, pkgName, typeName string) (*Type, error) {
	pkg, info, err := parsePkg(dir, pkgName)
	if err != nil {
		return nil, err
	}
	for id, obj := range info.Defs {
		if id.Obj == nil {
			continue
		}
		if id.Obj.Kind != ast.Typ || id.Name != typeName {
			continue
		}
		if !obj.Exported() {
			continue
		}
		return &Type{pkg, obj.Type()}, nil
	}
	return nil, fmt.Errorf("type %s.%s is not exist", pkg.Name(), typeName)
}

func (t *Type) Methods() ([]*types.Func, error) {
	m, ok := t.typ.(Methoder)
	if !ok {
		return nil, fmt.Errorf("no method!")
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
