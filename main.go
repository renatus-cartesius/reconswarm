/*
Copyright © 2025 renatuscartesius <cartesius.absolute@gmail.com>
*/
package main

import (
	"reconswarm/cmd"
	"reconswarm/internal/logging"
)

func main() {
	// Initialize logger
	if err := logging.InitLogger(); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}

	cmd.Execute()
}
