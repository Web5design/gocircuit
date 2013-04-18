package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// BuildConfig holds configuration parameters for the automated circuit app build system
type BuildConfig struct {
	Binary string // Has no effect. Use InstallConfig.Binary instead.
	Jail   string // Build jail path on build host

	AppRepo string // App repo URL
	AppSrc  string // App GOPATH relative to app repo; or empty string if app repo meant to be cloned inside a GOPATH
	WorkerPkg string // User program package that should be built as the circuit worker executable

	GoRepo    string
	RebuildGo bool   // Rebuild Go even if a newer version is not available
	Show      bool

	ZookeeperInclude string // Path to Zookeeper include files on build host
	ZookeeperLib     string // Path to Zookeeper library files on build host

	CircuitRepo string
	CircuitSrc  string

	Host       string // Host where build takes place
	PrefixPath string // PATH to pre-pend to default PATH environment on build host
	Tool       string // Build tool path on build host
	ShipDir    string // Local directory where built runtime binary and dynamic libraries will be delivered
}

func parseBuild() {
	bfile := os.Getenv("CIR_BUILD")
	if bfile == "" {
		return
	}
	Config.Build = &BuildConfig{}
	data, err := ioutil.ReadFile(bfile)
	if err != nil {
		Config.Build = nil
		fmt.Fprintf(os.Stderr, "Problem reading build config file %s (%s)", bfile, err)
		os.Exit(1)
	}
	if err = json.Unmarshal(data, Config.Build); err != nil {
		fmt.Fprintf(os.Stderr, "Problem parsing build file (%s)", err)
		os.Exit(1)
	}
}
