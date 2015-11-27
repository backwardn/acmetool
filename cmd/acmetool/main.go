package main

import (
	"bytes"
	"github.com/hlandau/acme/interaction"
	"github.com/hlandau/acme/notify"
	"github.com/hlandau/acme/redirector"
	"github.com/hlandau/acme/storage"
	"github.com/hlandau/degoutils/xlogconfig"
	"github.com/hlandau/xlog"
	"github.com/square/go-jose"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/hlandau/easyconfig.v1/adaptflag"
	"gopkg.in/hlandau/service.v2"
	"io/ioutil"
	"os"
	"path/filepath"
)

var log, Log = xlog.New("acmetool")

var (
	stateFlag = kingpin.Flag("state", "Path to the state directory (env: ACME_STATE_DIR)").
			Default(storage.RecommendedPath).
			Envar("ACME_STATE_DIR").
			PlaceHolder(storage.RecommendedPath).
			String()

	hooksFlag = kingpin.Flag("hooks", "Path to the notification hooks directory").
			Default(notify.DefaultHookPath).
			PlaceHolder(notify.DefaultHookPath).
			String()

	batchFlag = kingpin.Flag("batch", "Do not attempt interaction; useful for cron jobs").
			Bool()

	reconcileCmd = kingpin.Command("reconcile", "Reconcile ACME state").Default()

	wantCmd = kingpin.Command("want", "Add a target with one or more hostnames")
	wantArg = wantCmd.Arg("hostname", "hostnames for which a certificate is desired").Required().Strings()

	quickstartCmd = kingpin.Command("quickstart", "Interactively ask some getting started questions (recommended)")

	redirectorCmd      = kingpin.Command("redirector", "HTTP to HTTPS redirector with challenge response support")
	redirectorPathFlag = redirectorCmd.Flag("path", "Path to serve challenge files from").String()

	importJWKAccountCmd = kingpin.Command("import-jwk-account", "Import a JWK account key")
	importJWKURLArg     = importJWKAccountCmd.Arg("provider-url", "Provider URL (e.g. https://acme-v01.api.letsencrypt.org/directory)").Required().String()
	importJWKPathArg    = importJWKAccountCmd.Arg("private-key-file", "Path to private_key.json").Required().ExistingFile()

	importKeyCmd = kingpin.Command("import-key", "Import a certificate private key")
	importKeyArg = importKeyCmd.Arg("private-key-file", "Path to PEM-encoded private key").Required().ExistingFile()
)

func main() {
	adaptflag.Adapt()
	cmd := kingpin.Parse()
	xlogconfig.Init()

	if *batchFlag {
		interaction.NonInteractive = true
	}

	switch cmd {
	case "reconcile":
		reconcile()
	case "want":
		want()
		reconcile()
	case "quickstart":
		quickstart()
	case "redirector":
		runRedirector()
	case "import-jwk-account":
		importJWKAccount()
	case "import-key":
		importKey()
	}
}

func importJWKAccount() {
	s, err := storage.New(*stateFlag)
	log.Fatale(err, "storage")

	f, err := os.Open(*importJWKPathArg)
	log.Fatale(err, "cannot open private key file")
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	log.Fatale(err, "cannot read file")

	k := jose.JsonWebKey{}
	err = k.UnmarshalJSON(b)
	log.Fatale(err, "cannot unmarshal key")

	err = s.ImportAccountKey(*importJWKURLArg, k.Key)
	log.Fatale(err, "cannot import account key")
}

func importKey() {
	s, err := storage.New(*stateFlag)
	log.Fatale(err, "storage")

	f, err := os.Open(*importKeyArg)
	log.Fatale(err, "open")

	defer f.Close()

	err = s.ImportKey(f)
	log.Fatale(err, "cannot import key")
}

func reconcile() {
	s, err := storage.New(*stateFlag)
	log.Fatale(err, "storage")

	err = s.Reconcile()
	log.Fatale(err, "reconcile")
}

func want() {
	s, err := storage.New(*stateFlag)
	log.Fatale(err, "storage")

	tgt := storage.Target{
		Names: *wantArg,
	}

	err = s.AddTarget(tgt)
	log.Fatale(err, "add target")
}

func runRedirector() {
	rpath := *redirectorPathFlag
	if rpath == "" {
		rpath = determineWebroot()
	}

	service.Main(&service.Info{
		Name:          "acmetool",
		Description:   "acmetool HTTP redirector",
		DefaultChroot: rpath,
		NewFunc: func() (service.Runnable, error) {
			return redirector.New(redirector.Config{
				Bind:          ":80",
				ChallengePath: rpath,
			})
		},
	})
}

func determineWebroot() string {
	// don't use fdb for this, we don't need access to the whole db
	b, err := ioutil.ReadFile(filepath.Join(*stateFlag, "conf", "webroot-path"))
	if err == nil {
		b = bytes.TrimSpace(b)
		s := string(b)
		if s != "" {
			return s
		}
	}

	return "/var/run/acme/acme-challenge"
}

// © 2015 Hugo Landau <hlandau@devever.net>    MIT License
