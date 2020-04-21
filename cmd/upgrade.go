package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/google/go-github/v30/github"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var releaseTemplates = map[string]string{
	"darwin":  "Darwin",
	"linux":   "Linux",
	"windows": "Windows",
	"bit":     "Arm",
	"bitv6":   "Arm6",
	"bitv7":   "Arm7",
	"386":     "i386",
	"amd64":   "x86_64",
}

var ghclient = github.NewClient(nil)

func upgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades k9s",
		Long:  "Upgrades k9s to the latest available version",
		Run: func(cmd *cobra.Command, args []string) {
			doupgrade, id, artifactName, checksumArtifactID, remoteVersion := upgradeNeeded()
			if doupgrade {
				log.Info().Msgf("Upgrade required %v", doupgrade)
				downloadArifact(id, artifactName)
				downloadArifact(checksumArtifactID, "checksums.txt")
				if validateChecksums(artifactName) {
					f, _ := os.Open(artifactName)
					defer f.Close()
					extractTarGz(f)
					fmt.Println(fmt.Sprintf("Successfully upgraded from %s --> %s", version, remoteVersion))
				}
				cleanup(artifactName)
			}
		},
	}
}

func upgradeNeeded() (bool, int64, string, int64, string) {
	const fmat = "%-25s %s\n"
	localSemver, err := semver.NewVersion(version)
	if err != nil {
		log.Fatal().Err(err).Msgf("Unable to parse semantic version")
	}
	k9sArch := releaseTemplates[runtime.GOOS]
	k9sOs := releaseTemplates[runtime.GOARCH]
	k9sRelease := fmt.Sprintf("k9s_%s_%s.tar.gz", k9sArch, k9sOs)

	remoteVersion, artifacts := getLatestRemoteVersion()
	remoteSemver, _ := semver.NewVersion(remoteVersion)
	printTuple(fmat, "release", k9sRelease, -1)
	printTuple(fmat, "remoteVersion", remoteVersion, -1)
	printTuple(fmat, "installed version", localSemver.String(), -1)
	if remoteSemver.GreaterThan(localSemver) {
		return true, artifacts[k9sRelease], k9sRelease, artifacts["checksums.txt"], remoteVersion
	}
	return false, 0, "", 0, ""
}

func getLatestRemoteVersion() (string, map[string]int64) {
	rels, _, err := ghclient.Repositories.GetLatestRelease(context.Background(), "derailed", "k9s")
	releaseArtifacts := map[string]int64{}
	if err != nil {
		log.Error().Err(err).Msgf("Couldnt get latest version %v", err)
	}
	for _, r := range rels.Assets {
		releaseArtifacts[*r.Name] = *r.ID
	}

	return *rels.Name, releaseArtifacts
}

func downloadArifact(id int64, fileName string) {
	data, _, _ := ghclient.Repositories.DownloadReleaseAsset(context.Background(), "derailed", "k9s", id, http.DefaultClient)
	outFile, _ := os.Create(fileName)
	_, err := io.Copy(outFile, data)
	if err != nil {
		log.Error().Err(err).Msgf("Couldnt download artifact %v", err)
	}
	defer outFile.Close()
}

func validateChecksums(k9sRelease string) bool {
	md5sums := make(map[string]string)
	cf, _ := os.Open("checksums.txt")
	defer cf.Close()
	s := bufio.NewScanner(cf)
	for s.Scan() {
		z := strings.Fields(s.Text())
		md5sums[z[1]] = z[0]
	}

	f, _ := os.Open(k9sRelease)
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		log.Error().Err(err).Msgf("Couldn't calculate md5sum %v", err)
	}
	return md5sums[k9sRelease] == hex.EncodeToString(hash.Sum(nil))

}

func cleanup(k9sRelease string){
	base, _ := os.Getwd()
	fmt.Println(fmt.Sprintf("%s/checksums.txt",base))
	_ = os.Remove(fmt.Sprintf("%s/checksums.txt",base))
	err := os.Remove(fmt.Sprintf("%s/%s",base,k9sRelease))
	if err != nil {
		log.Error().Err(err).Msgf("Couldn't delete file %s", err)
	}
}


func extractTarGz(gzipStream io.Reader) {
	f, _ = os.Open(zipName)
	defer f.Close()
	// uncompressedStream, err := gzip.NewReader(gzipStream)
	uncompressedStream, err := gzip.NewReader(f)
	if err != nil {
		log.Error().Err(err).Msgf("ExtractTarGz: NewReader failed")
	}

	tarReader := tar.NewReader(uncompressedStream)
	
	defer uncompressedStream.Close()

	for true {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			log.Error().Err(err).Msgf("ExtractTarGz: Next() failed")
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(header.Name, 0755); err != nil {
				log.Error().Err(err).Msgf("ExtractTarGz: Mkdir() failed")
			}
		case tar.TypeReg:
			outFile, err := os.Create(header.Name)
			if err != nil {
				log.Error().Err(err).Msgf("ExtractTarGz: Create() failed: %s", err.Error())
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				log.Error().Err(err).Msgf("ExtractTarGz: Copy() failed: %s", err.Error())
			}
			outFile.Close()

		default:
			log.Error().Msgf(
				"ExtractTarGz: uknown type: %s in %s",
				header.Typeflag,
				header.Name)
		}

	}
}
