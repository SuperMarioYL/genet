package main

import (
	"os"

	"github.com/uc-package/genet/internal/genetcli"
)

func main() {
	if err := genetcli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
