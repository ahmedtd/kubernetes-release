/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"

	"k8s.io/release/pkg/gcp"
	"k8s.io/release/pkg/gh2gcs"
	"k8s.io/release/pkg/github"
	"k8s.io/release/pkg/log"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:               "gh2gcs --org kubernetes --repo release --bucket <bucket> --release-dir <release-dir> [--tags v0.0.0] [--include-prereleases] [--output-dir <temp-dir>] [--download-only] [--config <config-file>]",
	Short:             "gh2gcs uploads GitHub releases to Google Cloud Storage",
	Example:           "gh2gcs --org kubernetes --repo release --bucket k8s-staging-release-test --release-dir release --tags v0.0.0,v0.0.1",
	SilenceUsage:      true,
	SilenceErrors:     true,
	PersistentPreRunE: initLogging,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return checkRequiredFlags(cmd.Flags())
	},
	RunE: func(*cobra.Command, []string) error {
		return run(opts)
	},
}

type options struct {
	downloadOnly       bool
	includePrereleases bool
	org                string
	repo               string
	bucket             string
	releaseDir         string
	outputDir          string
	logLevel           string
	tags               []string
	configFilePath     string
}

var opts = &options{}

var (
	orgFlag                = "org"
	repoFlag               = "repo"
	tagsFlag               = "tags"
	configFlag             = "config"
	includePrereleasesFlag = "include-prereleases"
	bucketFlag             = "bucket"
	releaseDirFlag         = "release-dir"
	outputDirFlag          = "output-dir"
	downloadOnlyFlag       = "download-only"

	// requiredFlags only if the config flag is not set
	requiredFlags = []string{
		orgFlag,
		repoFlag,
		bucketFlag,
		releaseDirFlag,
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}

	if !rootCmd.Flags().Changed(outputDirFlag) {
		logrus.Infof("Cleaning temporary directory %s", opts.outputDir)
		os.RemoveAll(opts.outputDir)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&opts.org,
		orgFlag,
		"",
		"GitHub org/user",
	)

	rootCmd.PersistentFlags().StringVar(
		&opts.repo,
		repoFlag,
		"",
		"GitHub repo",
	)

	rootCmd.PersistentFlags().StringSliceVar(
		&opts.tags,
		tagsFlag,
		[]string{},
		"release tags to upload to GCS",
	)

	rootCmd.PersistentFlags().BoolVar(
		&opts.includePrereleases,
		includePrereleasesFlag,
		false,
		"specifies whether prerelease assets should be uploaded to GCS",
	)

	rootCmd.PersistentFlags().StringVar(
		&opts.bucket,
		bucketFlag,
		"",
		"GCS bucket to upload to",
	)

	rootCmd.PersistentFlags().StringVar(
		&opts.releaseDir,
		releaseDirFlag,
		"",
		"directory to upload to within the specified GCS bucket",
	)

	rootCmd.PersistentFlags().StringVar(
		&opts.outputDir,
		outputDirFlag,
		"",
		"local directory for releases to be downloaded to",
	)

	rootCmd.PersistentFlags().BoolVar(
		&opts.downloadOnly,
		downloadOnlyFlag,
		false,
		"only download the releases, do not push them to GCS. Requires the output-dir flag to also be set",
	)

	rootCmd.PersistentFlags().StringVar(
		&opts.logLevel,
		"log-level",
		"info",
		"the logging verbosity, either 'panic', 'fatal', 'error', 'warn', 'warning', 'info', 'debug' or 'trace'",
	)

	rootCmd.PersistentFlags().StringVar(
		&opts.configFilePath,
		configFlag,
		"",
		"config file to set all the branch/repositories the user wants to",
	)
}

func initLogging(*cobra.Command, []string) error {
	return log.SetupGlobalLogger(opts.logLevel)
}

func checkRequiredFlags(flags *pflag.FlagSet) error {
	if flags.Lookup(configFlag).Changed {
		return nil
	}

	checkRequiredFlags := []string{}
	flags.VisitAll(func(flag *pflag.Flag) {
		for _, requiredflag := range requiredFlags {
			if requiredflag == flag.Name && !flag.Changed {
				checkRequiredFlags = append(checkRequiredFlags, requiredflag)
			}
		}
	})

	if len(checkRequiredFlags) != 0 {
		return errors.New("Required flag(s) `" + strings.Join(checkRequiredFlags, ", ") + "` not set")
	}

	return nil
}

func run(opts *options) error {
	if err := opts.SetAndValidate(); err != nil {
		return errors.Wrap(err, "validating gh2gcs options")
	}

	if err := gcp.PreCheck(); err != nil {
		return errors.Wrap(err, "pre-checking for GCP package usage")
	}

	releaseConfigs := &gh2gcs.Config{}
	if opts.configFilePath != "" {
		logrus.Infof("Reading the config file %s...", opts.configFilePath)
		data, err := ioutil.ReadFile(opts.configFilePath)
		if err != nil {
			return errors.Wrap(err, "failed to read the file")
		}

		logrus.Info("Parsing the config...")
		err = yaml.UnmarshalStrict(data, &releaseConfigs)
		if err != nil {
			return errors.Wrap(err, "failed to decode the file")
		}

		for i, releaseConfig := range releaseConfigs.ReleaseConfigs {
			releaseConfigs.ReleaseConfigs[i].GCSCopyOptions = gh2gcs.CheckGCSCopyOptions(releaseConfig.GCSCopyOptions)
		}
	} else {
		// TODO: Expose certain GCSCopyOptions for user configuration
		releaseConfigs.ReleaseConfigs = append(releaseConfigs.ReleaseConfigs, gh2gcs.ReleaseConfig{
			Org:                opts.org,
			Repo:               opts.repo,
			Tags:               opts.tags,
			IncludePrereleases: opts.includePrereleases,
			GCSBucket:          opts.bucket,
			ReleaseDir:         opts.releaseDir,
			GCSCopyOptions:     gh2gcs.DefaultGCSCopyOptions,
		})
	}

	// Create a real GitHub API client
	gh := github.New()

	for _, releaseConfig := range releaseConfigs.ReleaseConfigs {
		if len(releaseConfig.Tags) == 0 {
			releaseTags, err := gh.GetReleaseTags(releaseConfig.Org, releaseConfig.Repo, releaseConfig.IncludePrereleases)
			if err != nil {
				return errors.Wrap(err, "getting release tags")
			}

			releaseConfig.Tags = releaseTags
		}

		if err := gh2gcs.DownloadReleases(&releaseConfig, gh, opts.outputDir); err != nil {
			return errors.Wrap(err, "downloading release assets")
		}
		logrus.Infof("Files downloaded to %s directory", opts.outputDir)

		if !opts.downloadOnly {
			if err := gh2gcs.Upload(&releaseConfig, gh, opts.outputDir); err != nil {
				return errors.Wrap(err, "uploading release assets to GCS")
			}
		}
	}

	return nil
}

// SetAndValidate sets some default options and verifies if options are valid
func (o *options) SetAndValidate() error {
	logrus.Info("Validating gh2gcs options...")

	if o.outputDir == "" {
		if opts.downloadOnly {
			return errors.Errorf("when %s flag is set you need to specify the %s", downloadOnlyFlag, outputDirFlag)
		}

		tmpDir, err := ioutil.TempDir("", "gh2gcs")
		if err != nil {
			return errors.Wrap(err, "creating temp directory")
		}

		o.outputDir = tmpDir
	}

	return nil
}
