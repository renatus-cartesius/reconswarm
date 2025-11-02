/*
Copyright © 2025 renatuscartesius <cartesius.absolute@gmail.com>
*/
package main

import (
	"reconswarm/cmd"
	"reconswarm/internal/logging"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	if err := logging.InitLogger(); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer func() {
		if err := logging.Sync(); err != nil {
			// Log sync error, but don't fail the application
			logging.Logger().Error("failed to sync logger on exit", zap.Error(err))
		}
	}()

	cmd.Execute()
}
