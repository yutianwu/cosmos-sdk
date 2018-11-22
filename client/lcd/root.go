package lcd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"
	tmserver "github.com/tendermint/tendermint/rpc/lib/server" // Import statik for light client stuff

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/keys"
	_ "github.com/cosmos/cosmos-sdk/client/lcd/statik"
	"github.com/cosmos/cosmos-sdk/codec"
	keybase "github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/cosmos/cosmos-sdk/server"
)

// RestServer represents the Light Client Rest server
type RestServer struct {
	Mux     *mux.Router
	CliCtx  context.CLIContext
	KeyBase keybase.Keybase
	Cdc     *codec.Codec

	log         log.Logger
	listener    net.Listener
	fingerprint string
}

// NewRestServer creates a new rest server instance
func NewRestServer(cdc *codec.Codec) *RestServer {
	r := mux.NewRouter()
	cliCtx := context.NewCLIContext().WithCodec(cdc)

	// Register version methods on the router
	r.HandleFunc("/version", CLIVersionRequestHandler).Methods("GET")
	r.HandleFunc("/node_version", NodeVersionRequestHandler(cliCtx)).Methods("GET")

	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout)).With("module", "rest-server")

	return &RestServer{
		Mux:    r,
		CliCtx: cliCtx,
		Cdc:    cdc,

		log: logger,
	}
}

func (rs *RestServer) setKeybase(kb keybase.Keybase) {
	// If a keybase is passed in, set it and return
	if kb != nil {
		rs.KeyBase = kb
		return
	}

	// Otherwise get the keybase and set it
	kb, err := keys.GetKeyBase() //XXX
	if err != nil {
		fmt.Printf("Failed to open Keybase: %s, exiting...", err)
		os.Exit(1)
	}
	rs.KeyBase = kb
}

// Start starts the rest server
func (rs *RestServer) Start(listenAddr string, sslHosts string,
	certFile string, keyFile string, maxOpen int, insecure bool) (err error) {

	server.TrapSignal(func() {
		err := rs.listener.Close()
		rs.log.Error("error closing listener", "err", err)
	})

	// TODO: re-enable insecure mode once #2715 has been addressed
	if insecure {
		fmt.Println(
			"Insecure mode is temporarily disabled, please locally generate an " +
				"SSL certificate to test. Support will be re-enabled soon!",
		)
		// listener, err = tmserver.StartHTTPServer(
		// 	listenAddr, handler, logger,
		// 	tmserver.Config{MaxOpenConnections: maxOpen},
		// )
		// if err != nil {
		// 	return
		// }
	} else {
		if certFile != "" {
			// validateCertKeyFiles() is needed to work around tendermint/tendermint#2460
			err = validateCertKeyFiles(certFile, keyFile)
			if err != nil {
				return err
			}

			//  cert/key pair is provided, read the fingerprint
			rs.fingerprint, err = fingerprintFromFile(certFile)
			if err != nil {
				return err
			}
		} else {
			// if certificate is not supplied, generate a self-signed one
			certFile, keyFile, rs.fingerprint, err = genCertKeyFilesAndReturnFingerprint(sslHosts)
			if err != nil {
				return err
			}

			defer func() {
				os.Remove(certFile)
				os.Remove(keyFile)
			}()
		}

		rs.listener, err = tmserver.StartHTTPAndTLSServer(
			listenAddr, rs.Mux,
			certFile, keyFile,
			rs.log,
			tmserver.Config{MaxOpenConnections: maxOpen},
		)
		if err != nil {
			return
		}

		rs.log.Info(rs.fingerprint)
		rs.log.Info("REST server started")
	}

	// logger.Info("REST server started")

	return nil
}

// ServeCommand will generate a long-running rest server
// (aka Light Client Daemon) that exposes functionality similar
// to the cli, but over rest
func (rs *RestServer) ServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rest-server",
		Short: "Start LCD (light-client daemon), a local REST server",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			rs.setKeybase(nil)
			// Start the rest server and return error if one exists
			err = rs.Start(
				viper.GetString(client.FlagListenAddr),
				viper.GetString(client.FlagSSLHosts),
				viper.GetString(client.FlagSSLCertFile),
				viper.GetString(client.FlagSSLKeyFile),
				viper.GetInt(client.FlagMaxOpenConnections),
				viper.GetBool(client.FlagInsecure))

			return err
		},
	}

	client.RegisterRestServerFlags(cmd)

	return cmd
}

func (rs *RestServer) registerSwaggerUI() {
	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}
	staticServer := http.FileServer(statikFS)
	rs.Mux.PathPrefix("/swagger-ui/").Handler(http.StripPrefix("/swagger-ui/", staticServer))
}

func validateCertKeyFiles(certFile, keyFile string) error {
	if keyFile == "" {
		return errors.New("a key file is required")
	}
	if _, err := os.Stat(certFile); err != nil {
		return err
	}
	if _, err := os.Stat(keyFile); err != nil {
		return err
	}
	return nil
}
