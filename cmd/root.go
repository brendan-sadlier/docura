package cmd

import (
	"github.com/spf13/cobra"
	"log"
)

var rootCmd = &cobra.Command{
	Use:   "docura",
	Short: "Docura is an AI powered documentation generator",
	Long:  `Docura is an AI powered documentation generator.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
