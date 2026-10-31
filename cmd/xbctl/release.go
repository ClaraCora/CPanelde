package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/ClaraCora/CPanelde/internal/releaseverify"
)

func runVerifyRelease(args []string) error {
	flags := flag.NewFlagSet("verify-release", flag.ContinueOnError)
	directory := flags.String("directory", "", "directory containing signed release metadata")
	artifact := flags.String("artifact", "", "signed asset name")
	file := flags.String("file", "", "downloaded asset path")
	version := flags.String("version", "", "required release version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *directory == "" || *artifact == "" || *file == "" || flags.NArg() != 0 {
		return errors.New("verify-release requires --directory, --artifact and --file")
	}
	return releaseverify.VerifyArtifact(*directory, *artifact, *file, *version)
}

func downloadSignedArtifact(artifact, version, destination string) (resultErr error) {
	directory, err := os.MkdirTemp(filepath.Dir(destination), ".signed-release-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	manifest, err := downloadReleaseMetadata(directory, version)
	if err != nil {
		return err
	}
	staged := filepath.Join(directory, artifact)
	if err := downloadFile(resolveDownloadURL(artifact, version), staged); err != nil {
		return err
	}
	if err := manifest.VerifyFile(artifact, staged); err != nil {
		return err
	}
	// Copy the verified bytes to the caller's staging file; never execute or
	// promote unauthenticated release contents.
	input, err := os.Open(staged)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err := output.Close(); resultErr == nil {
			resultErr = err
		}
		if resultErr != nil {
			os.Remove(destination)
		}
	}()
	_, err = io.Copy(output, input)
	return err
}

func resolveSignedReleaseVersion(version string) (string, error) {
	directory, err := os.MkdirTemp("", "corade-release-metadata-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	manifest, err := downloadReleaseMetadata(directory, version)
	if err != nil {
		return "", err
	}
	return manifest.VerifyVersion(filepath.Join(directory, "agent-version.txt"), version)
}

func downloadReleaseMetadata(directory, version string) (releaseverify.Manifest, error) {
	for _, item := range []struct {
		name  string
		limit int64
	}{
		{"release-manifest.sha256", 64 << 10},
		{"release-manifest.sha256.sig", 64},
		{"agent-version.txt", 256},
	} {
		if err := downloadFileBounded(resolveDownloadURL(item.name, version), filepath.Join(directory, item.name), item.limit); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "release-manifest.sha256"))
	if err != nil {
		return nil, err
	}
	signature, err := os.ReadFile(filepath.Join(directory, "release-manifest.sha256.sig"))
	if err != nil {
		return nil, err
	}
	manifest, err := releaseverify.Verify(data, signature)
	if err != nil {
		return nil, err
	}
	if _, err := manifest.VerifyVersion(filepath.Join(directory, "agent-version.txt"), version); err != nil {
		return nil, err
	}
	return manifest, nil
}
