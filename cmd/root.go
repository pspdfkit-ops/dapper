// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/rancher/dapper/file"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	VERSION string
	cfgFile string

	rootCmd = &cobra.Command{
		Use:   "dapper",
		Short: "dapper",
		Long: `Docker build wrapper

		Dockerfile variables

		DAPPER_SOURCE          The destination directory in the container to bind/copy the source
		DAPPER_CP              The location in the host to find the source
		DAPPER_OUTPUT          The files you want copied to the host in CP mode
		DAPPER_DOCKER_SOCKET   Whether the Docker socket should be bound in
		DAPPER_RUN_ARGS        Args to add to the docker run command when building
		DAPPER_ENV             Env vars that should be copied into the build
		DAPPER_VOLUMES         Volumes that should be mounted on docker run`,
		Run: func(cmd *cobra.Command, args []string) {

			if viper.GetBool("version") {
				fmt.Printf("%s version %s\n", cmd.Name(), VERSION)
				os.Exit(0)
			}
			if viper.GetBool("debug") {
				log.SetLevel(log.DebugLevel)
			}

			if directory := viper.GetString("directory"); directory != "" {
				if err := os.Chdir(directory); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to change to directory %s: %v\n", directory, err)
					os.Exit(1)
				}
			}

			dapperFile, err := file.Lookup(viper.GetString("file"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				os.Exit(1)
			}

			dapperFile.Mode = viper.GetString("mode")
			dapperFile.Socket = viper.GetBool("socket")
			dapperFile.NoOut = viper.GetBool("no-out")
			dapperFile.Quiet = viper.GetBool("quiet")
			dapperFile.Keep = viper.GetBool("keep")
			dapperFile.NoContext = viper.GetBool("no-context")
			dapperFile.MapUser = viper.GetBool("map-user")

			if dapperFile.NoContext {
				dapperFile.Mode = "bind"
			}

			// todo extra cmd
			if viper.GetBool("shell") {
				dapperFile.Shell(args)
			}

			// todo extra cmd
			if viper.GetBool("build") {
				dapperFile.Build(args)
			}

			if viper.GetBool("generate-bash-completion") {
				cmd.GenBashCompletion(os.Stdout)
				os.Exit(0)
			}

			dapperFile.Run(args)
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(version string) {
	VERSION = version

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $PWD/dapper.yaml)")

	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Print debugging")
	rootCmd.PersistentFlags().StringP("file", "f", "Dockerfile.dapper", "Dockerfile to build from")
	rootCmd.PersistentFlags().StringP("mode", "m", "auto", "Execution mode for Dapper bind/cp/auto")
	rootCmd.PersistentFlags().StringP("directory", "C", ".", "The directory in which to run, --file is relative to this")
	rootCmd.PersistentFlags().BoolP("shell", "s", false, "Launch a shell")
	rootCmd.PersistentFlags().BoolP("socket", "k", false, "Bind in the Docker socket")
	rootCmd.PersistentFlags().Bool("build", false, "Perform a build")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Make Docker build quieter")
	rootCmd.PersistentFlags().Bool("keep", false, "Don't remove the container that was used to build")
	rootCmd.PersistentFlags().BoolP("no-context", "X", false, "send Dockerfile via stdin to docker build command")
	rootCmd.PersistentFlags().BoolP("no-out", "O", false, "Do not copy the output back (in --mode cp)")
	rootCmd.PersistentFlags().BoolP("map-user", "u", false, "Map UID/GID from dapper process to docker run")
	rootCmd.PersistentFlags().Bool("generate-bash-completion", false, "Generates Bash completion script to Stdout")
	rootCmd.PersistentFlags().BoolP("version", "v", false, "Show version")

	viper.BindPFlags(rootCmd.PersistentFlags())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else if os.Getenv("DAPPER_CONFIG") != "" {
		viper.SetConfigFile(os.Getenv("DAPPER_CONFIG"))
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// current directory
		viper.AddConfigPath(".")

		// parent direcotrx
		viper.AddConfigPath("..")

		// home directory
		viper.AddConfigPath(home)

		// config file prefix.
		// -> dapper{.yaml|.json|.toml}
		viper.SetConfigName("dapper")
	}

	// environment variables have to be prefixed with DAPPER_
	viper.SetEnvPrefix("DAPPER")

	// DAPPER_NO_CONTEXT => no-context
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
