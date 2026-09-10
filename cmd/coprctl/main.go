package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Furyfree/copr/internal/packaging"
)

func run(ctx context.Context, args []string) error {
	f := flag.NewFlagSet("coprctl", flag.ContinueOnError)
	root := f.String("root", ".", "repository root")
	if err := f.Parse(args); err != nil {
		return err
	}
	args = f.Args()
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	b := packaging.New(abs)
	if len(args) == 0 {
		return fmt.Errorf("usage: coprctl [--root DIR] prepare PACKAGE OUTDIR | srpm --spec FILE --outdir DIR | project PACKAGE | publish PACKAGE SRPM | verify-specs [PACKAGE]")
	}
	switch args[0] {
	case "prepare":
		if len(args) != 3 {
			return fmt.Errorf("prepare requires PACKAGE OUTDIR")
		}
		path, err := b.Prepare(ctx, args[1], args[2])
		if err == nil {
			fmt.Println(path)
		}
		return err
	case "srpm":
		flags := flag.NewFlagSet("srpm", flag.ContinueOnError)
		spec := flags.String("spec", "", "RPM spec")
		out := flags.String("outdir", "", "SRPM output")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		absolute, err := filepath.Abs(*spec)
		if err != nil {
			return err
		}
		config, err := packaging.Load(abs)
		if err != nil {
			return err
		}
		for name := range config.Projects {
			if absolute == filepath.Join(abs, "packages", name, name+".spec") {
				path, err := b.Prepare(ctx, name, *out)
				if err == nil {
					fmt.Println(path)
				}
				return err
			}
		}
		path, err := b.SRPM(ctx, *spec, *out)
		if err == nil {
			fmt.Println(path)
		}
		return err
	case "project":
		if len(args) != 2 {
			return fmt.Errorf("project requires PACKAGE")
		}
		return packaging.NewPublisher(abs).Publish(ctx, "project", args[1], "")
	case "publish":
		if len(args) != 3 {
			return fmt.Errorf("publish requires PACKAGE SRPM")
		}
		return packaging.NewPublisher(abs).Publish(ctx, "build", args[1], args[2])
	case "verify-specs":
		name := ""
		if len(args) > 2 {
			return fmt.Errorf("verify-specs accepts one package")
		}
		if len(args) == 2 {
			name = args[1]
		}
		return b.VerifySpecs(ctx, name)
	default:
		return fmt.Errorf("unknown operation %q", args[0])
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "coprctl:", err)
		os.Exit(1)
	}
}
