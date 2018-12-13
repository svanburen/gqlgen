package codegen

import (
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/model"
	"github.com/99designs/gqlgen/codegen/templates"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/ast"
)

type Generator struct {
	*config.Config
	schema    *ast.Schema       `yaml:"-"`
	SchemaStr map[string]string `yaml:"-"`
}

func New(cfg *config.Config) (*Generator, error) {
	g := &Generator{Config: cfg}

	var err error
	g.schema, g.SchemaStr, err = cfg.LoadSchema()
	if err != nil {
		return nil, err
	}

	err = cfg.Check()
	if err != nil {
		return nil, err
	}

	return g, nil
}

func (cfg *Generator) Generate() error {
	_ = syscall.Unlink(cfg.Exec.Filename)
	_ = syscall.Unlink(cfg.Model.Filename)

	models, err := model.Generate(cfg.Config, cfg.schema)
	if err != nil {
		return errors.Wrap(err, "model generation failed")
	}

	for name, newCfg := range models {
		modelCfg := cfg.Models[name]
		modelCfg.Model = newCfg.Model
		cfg.Models[name] = modelCfg
	}

	build, err := cfg.bind()
	if err != nil {
		return errors.Wrap(err, "exec plan failed")
	}

	if err = templates.RenderToFile("generated.gotpl", cfg.Exec.Filename, build); err != nil {
		return err
	}

	if cfg.Resolver.IsDefined() {
		if err := cfg.GenerateResolver(); err != nil {
			return errors.Wrap(err, "generating resolver failed")
		}
	}

	if err := cfg.ValidateGeneratedCode(); err != nil {
		return errors.Wrap(err, "validation failed")
	}

	return nil
}

func (cfg *Generator) GenerateServer(filename string) error {
	serverFilename := abs(filename)
	serverBuild := cfg.server(filepath.Dir(serverFilename))

	if _, err := os.Stat(serverFilename); os.IsNotExist(errors.Cause(err)) {
		err = templates.RenderToFile("server.gotpl", serverFilename, serverBuild)
		if err != nil {
			return errors.Wrap(err, "generate server failed")
		}
	} else {
		log.Printf("Skipped server: %s already exists\n", serverFilename)
	}
	return nil
}

func (cfg *Generator) GenerateResolver() error {
	resolverBuild, err := cfg.resolver()
	if err != nil {
		return errors.Wrap(err, "resolver build failed")
	}
	filename := cfg.Resolver.Filename

	if resolverBuild.ResolverFound {
		log.Printf("Skipped resolver: %s.%s already exists\n", cfg.Resolver.ImportPath(), cfg.Resolver.Type)
		return nil
	}

	if _, err := os.Stat(filename); os.IsNotExist(errors.Cause(err)) {
		if err := templates.RenderToFile("resolver.gotpl", filename, resolverBuild); err != nil {
			return err
		}
	} else {
		log.Printf("Skipped resolver: %s already exists\n", filename)
	}

	return nil
}