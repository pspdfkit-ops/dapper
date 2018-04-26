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

	"github.com/rancher/dapper/file"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tarent/logrus"
)

var (
	VERSION      string
	cfgFile      string
	debug        bool
	directory    string
	shell        bool
	build        bool
	filename     string
	mode         string
	socket       bool
	no_out       bool
	quiet        bool
	keep         bool
	no_context   bool
	show_version bool

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
		DAPPER_ENV             Env vars that should be copied into the build`,
		Run: func(cmd *cobra.Command, args []string) {
			if show_version {
				fmt.Printf("%s version %s\n", cmd.Name(), VERSION)
				os.Exit(0)
			}

			if debug {
				logrus.SetLevel(logrus.DebugLevel)
			}

			if err := os.Chdir(directory); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to change to directory %s: %v\n", directory, err)
				os.Exit(1)
			}

			dapperFile, err := file.Lookup(filename)
			if err != nil {
				fmt.Fprint(os.Stderr, err)
				os.Exit(1)
			}

			dapperFile.Mode = mode
			dapperFile.Socket = socket
			dapperFile.NoOut = no_out
			dapperFile.Quiet = quiet
			dapperFile.Keep = keep
			dapperFile.NoContext = no_context

			// todo extra cmd
			if shell {
				dapperFile.Shell(args)
			}

			// todo extra cmd
			if build {
				dapperFile.Build(args)
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $PWD/.dapper.yaml)")

	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Print debugging")
	rootCmd.PersistentFlags().StringVarP(&filename, "file", "f", "Dockerfile.dapper", "Dockerfile to build from")
	rootCmd.PersistentFlags().StringVarP(&mode, "mode", "m", "auto", "Execution mode for Dapper bind/cp/auto")
	rootCmd.PersistentFlags().StringVarP(&directory, "directory", "C", ".", "The directory in which to run, --file is relative to this")
	rootCmd.PersistentFlags().BoolVarP(&shell, "shell", "s", false, "Launch a shell")
	rootCmd.PersistentFlags().BoolVarP(&socket, "socket", "k", false, "Bind in the Docker socket")
	rootCmd.PersistentFlags().BoolVar(&build, "build", false, "Perform a build")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Make Docker build quieter")
	rootCmd.PersistentFlags().BoolVar(&keep, "keep", false, "Don't remove the container that was used to build")
	rootCmd.PersistentFlags().BoolVarP(&no_context, "no-context", "X", false, "send Dockerfile via stdin to docker build command")
	rootCmd.PersistentFlags().BoolVarP(&no_out, "no-out", "O", false, "Do not copy the output back (in --mode cp)")
	rootCmd.PersistentFlags().BoolVarP(&show_version, "version", "v", false, "Show version")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		//home, err := homedir.Dir()
		//if err != nil {
		//	fmt.Println(err)
		//	os.Exit(1)
		//}

		// Search config in home directory with name ".tmp" (without extension).
		//viper.AddConfigPath(home)
		viper.SetConfigName(".dapper")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
