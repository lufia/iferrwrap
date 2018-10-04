package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
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
	if err := codegen(pkgName, typeName); err != nil {
		log.Fatalln(err)
	}
}

func codegen(pkgName, typeName string) error {
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
	meths, err := t.Methods()
	if err != nil {
		return err
	}
	for _, m := range meths {
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return fmt.Errorf("%v: don't have signature?", m)
		}
		rv := sig.Results()
		if rv == nil || rv.Len() == 0 {
			log.Println(m, ": void")
			continue
		}
		if !isErrorType(rv.At(rv.Len() - 1).Type()) {
		}
		log.Println(m.Name(), sig, ok)
	}
	return nil
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
	return nil, fmt.Errorf("type %s.%s is not exist", pkg.Name, typeName)
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
