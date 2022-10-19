package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/mod/modfile"
)

//go:embed skel
//go:embed skel/views/*.html
//go:embed skel/views/layouts/*.html
var skel embed.FS

type initConfig struct {
	Binary string

	WithDB         bool
	WithSession    bool
	WithTranslator bool
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	var initConf initConfig
	initCmd := flag.NewFlagSet("init", flag.ExitOnError)
	initCmd.BoolVar(&initConf.WithDB, "with-db", false, "with database")
	initCmd.BoolVar(&initConf.WithSession, "with-session", false, "with session")
	initCmd.BoolVar(&initConf.WithTranslator, "with-translator", false, "with translator")

	prg := filepath.Base(args[0])

	if len(args) < 2 {
		return errors.New(help(prg))
	}

	switch args[1] {

	case "init":
		initCmd.Parse(args[2:])
		return initialize(initConf)
	default:
		return errors.New(help(prg))
	}
}

func help(prg string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s COMMAND\n\n", prg)
	fmt.Fprintf(&b, "Helper to instantiate a bow web project\n\n")
	fmt.Fprintf(&b, "Commands:\n")
	fmt.Fprintf(&b, "  init       Initialize a new bow project\n\n")
	fmt.Fprintf(&b, "Run %s COMMAND -h for more information on a command", prg)
	return b.String()
}

func initialize(conf initConfig) error {
	var err error

	fmt.Println("adding dependencies to go.mod")

	conf.Binary, err = requireModule()
	if err != nil {
		return err
	}

	fmt.Println("generating all the necessary files")

	err = createSkeleton(conf)
	if err != nil {
		return fmt.Errorf("cannot init: %w", err)
	}

	fmt.Println("project is ready")

	return nil
}

// requireModule imports the bow module into go.mod
// and returns the binary name computed from the module path.
func requireModule() (string, error) {
	if _, err := os.Stat("go.mod"); errors.Is(err, os.ErrNotExist) {
		return "", errors.New("please init a go module with 'go mod init' first")
	}

	f, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}

	modFile, err := modfile.Parse("go.mod", f, nil)
	if err != nil {
		return "", err
	}

	modFile.AddNewRequire("github.com/lobre/bow", "latest", false)
	modFile.AddNewRequire("github.com/julienschmidt/httprouter", "latest", false)

	modBytes, err := modFile.Format()
	if err != nil {
		return "", err
	}

	if err := os.WriteFile("go.mod", modBytes, 0644); err != nil {
		return "", err
	}

	return filepath.Base(modFile.Module.Mod.Path), nil
}

func createSkeleton(conf initConfig) error {
	err := fs.WalkDir(skel, "skel", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// skip for skel root folder
		if path == "skel" {
			return nil
		}

		// compute local path without skel/
		parts := strings.Split(path, "/")
		localPath := filepath.Join(parts[1:]...)
		rootPath := parts[1]

		// skip migrations folder if no db
		if !conf.WithDB && rootPath == "migrations" {
			return nil
		}

		// skip translations folder is no translator
		if !conf.WithTranslator && rootPath == "translations" {
			return nil
		}

		if d.IsDir() {
			if err := os.MkdirAll(localPath, os.ModeDir|0755); err != nil {
				return err
			}
			return nil
		}

		// to allow having a gitignore skeleton file that
		// is not taken into account by git in this repo
		if localPath == "gitignore" {
			localPath = ".gitignore"
		}

		if _, err := os.Stat(localPath); err == nil {
			fmt.Printf("project already contains the file %s, skipping...\n", localPath)
			return nil
		}

		tmpl, err := template.New(filepath.Base(path)).Delims("{%", "%}").ParseFS(skel, path)
		if err != nil {
			return err
		}

		fmt.Printf("creating %s\n", localPath)

		f, err := os.Create(localPath)
		if err != nil {
			return err
		}

		if err := tmpl.Execute(f, conf); err != nil {
			return err
		}

		return f.Close()
	})
	if err != nil {
		return err
	}

	return nil
}
