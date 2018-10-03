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

type methoder interface {
	Method(i int) *types.Func
	NumMethods() int
}

func main() {
	log.SetFlags(0)
	log.SetPrefix(filepath.Base(os.Args[0]) + ": ")

	ctx := build.Default
	//ctx.BuildTags = []string{}
	pkg, err := ctx.Import("database/sql", ".", 0)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(pkg.Name, pkg.Dir)
	t, err := lookupType(pkg, "DB")
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(t.obj.Type())
	m, ok := t.obj.Type().(methoder)
	if !ok {
		log.Fatal("no method!")
	}
	for i := 0; i < m.NumMethods(); i++ {
		f := m.Method(i)
		if !f.Exported() {
			continue
		}
		log.Println(f.Name(), f.Type())
	}
}

type Type struct {
	pkg *types.Package
	obj types.Object
}

func lookupType(pkg *build.Package, typeName string) (*Type, error) {
	fs := token.NewFileSet()
	pkgs, err := parser.ParseDir(fs, pkg.Dir, nil, 0)
	if err != nil {
		return nil, err
	}
	p, ok := pkgs[pkg.Name]
	if !ok {
		return nil, fmt.Errorf("package %s is not exist", pkg.Name)
	}

	cfg := types.Config{
		Importer: importer.For("source", nil),
	}
	files := make([]*ast.File, 0, len(p.Files))
	for _, f := range p.Files {
		files = append(files, f)
	}

	info := types.Info{
		Defs: make(map[*ast.Ident]types.Object),
	}
	typesPkg, err := cfg.Check(pkg.Dir, fs, files, &info)
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
		return &Type{typesPkg, obj}, nil
	}
	return nil, fmt.Errorf("type %s.%s is not exist", pkg.Name, typeName)
}
