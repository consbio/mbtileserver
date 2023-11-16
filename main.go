package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/evalphobia/logrus_sentry"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	mbtiles "github.com/brendan-ward/mbtiles-go"
	"github.com/consbio/mbtileserver/handlers"
)

var rootCmd = &cobra.Command{
	Use:   "mbtileserver",
	Short: "Serve tiles from mbtiles files",
	Run: func(cmd *cobra.Command, args []string) {
		// default to listening on all interfaces
		if host == "" {
			host = "0.0.0.0"
		}
		// if port is not provided by user (after parsing command line args), use defaults
		if port == -1 {
			if len(certificate) > 0 || autotls {
				port = 443
			} else {
				port = 8000
			}
		}

		if enableReloadSignal {
			if isChild := os.Getenv("MBTS_IS_CHILD"); isChild != "" {
				serve()
			} else {
				supervise()
			}
		} else {
			serve()
		}
	},
}

var (
	host                string
	port                int
	tilePath            string
	certificate         string
	privateKey          string
	rootURLStr          string
	domain              string
	secretKey           string
	sentryDSN           string
	verbose             bool
	autotls             bool
	redirect            bool
	enableReloadSignal  bool
	enableReloadFSWatch bool
	generateIDs         bool
	enableArcGIS        bool
	disablePreview      bool
	disableTileJSON     bool
	disableServiceList  bool
	tilesOnly           bool
	basemapStyleURL     string
	basemapTilesURL     string
)

func init() {
	flags := rootCmd.Flags()
	flags.StringVar(&host, "host", "0.0.0.0", "IP address to listen on. Default is all interfaces.")
	flags.IntVarP(&port, "port", "p", -1, "Server port. Default is 443 if --cert or --tls options are used, otherwise 8000.")
	flags.StringVarP(&tilePath, "dir", "d", "./tilesets", "Directory containing mbtiles files.  Can be a comma-delimited list of directories.")
	flags.BoolVarP(&generateIDs, "generate-ids", "", false, "Automatically generate tileset IDs instead of using relative path")
	flags.StringVarP(&certificate, "cert", "c", "", "X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.")
	flags.StringVarP(&privateKey, "key", "k", "", "TLS private key")
	flags.StringVar(&rootURLStr, "root-url", "/services", "Root URL of services endpoint")
	flags.StringVar(&domain, "domain", "", "Domain name of this server.  NOTE: only used for AutoTLS.")
	flags.StringVarP(&secretKey, "secret-key", "s", "", "Shared secret key used for HMAC request authentication")
	flags.BoolVarP(&autotls, "tls", "t", false, "Auto TLS via Let's Encrypt")
	flags.BoolVarP(&redirect, "redirect", "r", false, "Redirect HTTP to HTTPS")

	flags.BoolVarP(&enableArcGIS, "enable-arcgis", "", false, "Enable ArcGIS Mapserver endpoints")
	flags.BoolVarP(&enableReloadFSWatch, "enable-fs-watch", "", false, "Enable reloading of tilesets by watching filesystem")
	flags.BoolVarP(&enableReloadSignal, "enable-reload-signal", "", false, "Enable graceful reload using HUP signal to the server process")

	flags.BoolVarP(&disablePreview, "disable-preview", "", false, "Disable map preview for each tileset (enabled by default)")
	flags.BoolVarP(&disableTileJSON, "disable-tilejson", "", false, "Disable TileJSON endpoint for each tileset (enabled by default)")
	flags.BoolVarP(&disableServiceList, "disable-svc-list", "", false, "Disable services list endpoint (enabled by default)")
	flags.BoolVarP(&tilesOnly, "tiles-only", "", false, "Only enable tile endpoints (shortcut for --disable-svc-list --disable-tilejson --disable-preview)")

	flags.StringVar(&sentryDSN, "dsn", "", "Sentry DSN")

	flags.StringVar(&basemapStyleURL, "basemap-style-url", "", "Basemap style URL for preview endpoint (can include authorization token parameter if required by host)")
	flags.StringVar(&basemapTilesURL, "basemap-tiles-url", "", "Basemap raster tiles URL pattern for preview endpoint (can include authorization token parameter if required by host): https://some.host/{z}/{x}/{y}.png")

	flags.BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")

	if env := os.Getenv("HOST"); env != "" {
		host = env
	}

	if env := os.Getenv("PORT"); env != "" {
		p, err := strconv.Atoi(env)
		if err != nil {
			log.Fatalln("PORT must be a number")
		}
		port = p
	}

	if env := os.Getenv("TILE_DIR"); env != "" {
		tilePath = env
	}

	if env := os.Getenv("GENERATE_IDS"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("GENERATE_IDS must be a bool(true/false)")
		}
		generateIDs = p
	}

	if env := os.Getenv("TLS_CERT"); env != "" {
		certificate = env
	}

	if env := os.Getenv("TLS_PRIVATE_KEY"); env != "" {
		privateKey = env
	}

	if env := os.Getenv("ROOT_URL"); env != "" {
		rootURLStr = env
	}

	if env := os.Getenv("DOMAIN"); env != "" {
		domain = env
	}
	if secretKey == "" {
		secretKey = os.Getenv("HMAC_SECRET_KEY")
	}

	if env := os.Getenv("AUTO_TLS"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("AUTO_TLS must be a bool(true/false)")
		}
		autotls = p
	}

	if env := os.Getenv("REDIRECT"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("REDIRECT must be a bool(true/false)")
		}
		redirect = p
	}

	if env := os.Getenv("DSN"); env != "" {
		sentryDSN = env
	}

	if env := os.Getenv("ENABLE_ARCGIS"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("ENABLE_ARCGIS must be a bool(true/false)")
		}
		enableArcGIS = p
	}

	if env := os.Getenv("ENABLE_FS_WATCH"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("ENABLE_FS_WATCH must be a bool(true/false)")
		}
		enableReloadFSWatch = p
	}

	if env := os.Getenv("ENABLE_RELOAD_SIGNAL"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("ENABLE_RELOAD_SIGNAL must be a bool(true/false)")
		}
		enableReloadSignal = p
	}

	if env := os.Getenv("VERBOSE"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("VERBOSE must be a bool(true/false)")
		}
		verbose = p
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func serve() {
	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if len(sentryDSN) > 0 {
		hook, err := logrus_sentry.NewSentryHook(sentryDSN, []log.Level{
			log.PanicLevel,
			log.FatalLevel,
			log.ErrorLevel,
			log.WarnLevel,
		})
		if err != nil {
			log.Fatalln(err)
		}
		hook.Timeout = 30 * time.Second // allow up to 30 seconds for Sentry to respond
		log.AddHook(hook)
		log.Debugln("Added logging hook for Sentry")
	}

	if tilesOnly {
		disableServiceList = true
		disableTileJSON = true
		disablePreview = true
	}

	if disablePreview {
		basemapStyleURL = ""
		basemapTilesURL = ""
	}

	if !strings.HasPrefix(rootURLStr, "/") {
		log.Fatalln("Value for --root-url must start with \"/\"")
	}
	if strings.HasSuffix(rootURLStr, "/") {
		log.Fatalln("Value for --root-url must not end with \"/\"")
	}

	rootURL, err := url.Parse(rootURLStr)
	if err != nil {
		log.Fatalf("Could not parse --root-url value %q\n", rootURLStr)
	}

	certExists := len(certificate) > 0
	keyExists := len(privateKey) > 0
	domainExists := len(domain) > 0

	if certExists != keyExists {
		log.Fatalln("Both certificate and private key are required to use SSL")
	}

	if autotls && !domainExists {
		log.Fatalln("Domain is required to use auto TLS")
	}

	if (certExists || autotls) && port != 443 {
		log.Warnln("Port 443 should be used for TLS")
	}

	if redirect && !(certExists || autotls) {
		log.Fatalln("Certificate or tls options are required to use redirect")
	}

	if len(secretKey) > 0 {
		log.Infoln("An HMAC request authorization key was set.  All incoming must be signed.")
	}

	generateID := func(filename string, baseDir string) (string, error) {
		if generateIDs {
			return handlers.SHA1ID(filename), nil
		} else {
			return handlers.RelativePathID(filename, baseDir)
		}
	}

	svcSet, err := handlers.New(&handlers.ServiceSetConfig{
		RootURL:           rootURL,
		ErrorWriter:       &errorLogger{log: log.New()},
		EnableServiceList: !disableServiceList,
		EnableTileJSON:    !disableTileJSON,
		EnablePreview:     !disablePreview,
		EnableArcGIS:      enableArcGIS,
		BasemapStyleURL:   basemapStyleURL,
		BasemapTilesURL:   basemapTilesURL,
	})
	if err != nil {
		log.Fatalln("Could not construct ServiceSet")
	}

	for _, path := range strings.Split(tilePath, ",") {
		// Discover all tilesets
		log.Infof("Searching for tilesets in %v\n", path)
		filenames, err := mbtiles.FindMBtiles(path)
		if err != nil {
			log.Errorf("Unable to list mbtiles in '%v': %v\n", path, err)
		}
		if len(filenames) == 0 {
			log.Errorf("No tilesets found in %s", path)
		}

		// Register all tilesets
		for _, filename := range filenames {
			id, err := generateID(filename, path)
			if err != nil {
				log.Errorf("Could not generate ID for tileset: %q", filename)
				continue
			}

			err = svcSet.AddTileset(filename, id)
			if err != nil {
				log.Errorf("Could not add tileset for %q with ID %q\n%v", filename, id, err)
			}
		}
	}

	// print number of services
	log.Infof("Published %v services", svcSet.Size())

	// watch filesystem for changes to tilesets
	if enableReloadFSWatch {
		watcher, err := NewFSWatcher(svcSet, generateID)
		if err != nil {
			log.Fatalln("Could not construct filesystem watcher")
		}
		defer watcher.Close()

		for _, path := range strings.Split(tilePath, ",") {
			log.Infof("Watching %v\n", path)
			err = watcher.WatchDir((path))
			if err != nil {
				// If we cannot enable file watching, then this should be a fatal
				// error during server startup
				log.Fatalln("Could not enable filesystem watcher in", path, err)
			}
		}
	}

	e := echo.New()
	e.HideBanner = true
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// log all requests if verbose mode
	if verbose {
		e.Use(middleware.Logger())
	}

	// setup auth middleware if secret key is set
	if secretKey != "" {
		hmacAuth := handlers.HMACAuthMiddleware(secretKey, svcSet)
		e.Use(echo.WrapMiddleware(hmacAuth))
	}

	// Get HTTP.Handler for the service set, and wrap for use in echo
	e.GET("/*", echo.WrapHandler(svcSet.Handler()))

	// Start the server
	fmt.Println("\n--------------------------------------")
	fmt.Println("Use Ctrl-C to exit the server")
	fmt.Println("--------------------------------------")

	// If starting TLS on 443, start server on port 80 too
	if redirect {
		e.Pre(middleware.HTTPSRedirect())
		if port == 443 {
			go func(c *echo.Echo) {
				fmt.Printf("HTTP server with redirect started on %v:80\n", host)
				log.Fatal(e.Start(fmt.Sprintf("%v:%v", host, 80)))
			}(e)
		}
	}

	var listener net.Listener

	if enableReloadSignal {
		f := os.NewFile(3, "")
		listener, err = net.FileListener(f)
	} else {
		listener, err = net.Listen("tcp", fmt.Sprintf("%v:%v", host, port))
	}

	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{Handler: e}

	// Listen for SIGHUP (graceful shutdown)
	go func(e *echo.Echo) {
		if !enableReloadSignal {
			return
		}

		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)

		<-hup

		context, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		e.Shutdown(context)

		os.Exit(0)
	}(e)

	switch {
	case certExists:
		{
			log.Debug("Starting HTTPS using provided certificate")
			if _, err := os.Stat(certificate); os.IsNotExist(err) {
				log.Fatalf("Could not find certificate file: %s\n", certificate)
			}
			if _, err := os.Stat(privateKey); os.IsNotExist(err) {
				log.Fatalf("Could not find private key file: %s\n", privateKey)
			}

			fmt.Printf("HTTPS server started on %v:%v\n", host, port)
			log.Fatal(server.ServeTLS(listener, certificate, privateKey))
		}
	case autotls:
		{
			log.Debug("Starting HTTPS using Let's Encrypt")

			// Setup certificate cache directory and TLS config
			e.AutoTLSManager.Cache = autocert.DirCache(".certs")
			e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(domain)

			server.TLSConfig = new(tls.Config)
			server.TLSConfig.GetCertificate = e.AutoTLSManager.GetCertificate
			server.TLSConfig.NextProtos = append(server.TLSConfig.NextProtos, acme.ALPNProto)
			if !e.DisableHTTP2 {
				server.TLSConfig.NextProtos = append(server.TLSConfig.NextProtos, "h2")
			}

			tlsListener := tls.NewListener(listener, server.TLSConfig)

			fmt.Printf("HTTPS server started on %v:%v\n", host, port)
			log.Fatal(server.Serve(tlsListener))
		}
	default:
		{
			fmt.Printf("HTTP server started on %v:%v\n", host, port)
			log.Fatal(server.Serve(listener))
		}
	}
}

// The main process forks and manages a sub-process for graceful reloading
func supervise() {

	listener, err := net.Listen("tcp", fmt.Sprintf("%v:%v", host, port))
	if err != nil {
		log.Fatal(err)
	}

	createFork := func() *exec.Cmd {
		environment := append(os.Environ(), "MBTS_IS_CHILD=true")
		path, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}

		listenerFile, err := listener.(*net.TCPListener).File()
		if err != nil {
			log.Fatal(err)
		}

		cmd := exec.Command(path, os.Args...)
		cmd.Env = environment
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.ExtraFiles = []*os.File{listenerFile}

		cmd.Start()

		return cmd
	}

	killFork := func(cmd *exec.Cmd) {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		cmd.Process.Signal(syscall.SIGHUP) // Signal fork to shut down gracefully

		select {
		case <-time.After(30 * time.Second): // Give fork 30 seconds to shut down gracefully
			if err := cmd.Process.Kill(); err != nil {
				log.Errorf("Could not kill child process: %v", err)
			}
		case <-done:
			return
		}
	}

	var child *exec.Cmd
	shutdown := false

	// Graceful shutdown on Ctrl + C
	go func() {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

		<-interrupt

		shutdown = true
		fmt.Println("\nShutting down...")

		if child != nil {
			killFork(child)
			os.Exit(0)
		}
	}()

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)

	for {
		if child != nil {
			killFork(child)
		}

		if shutdown {
			break
		}

		cmd := createFork()

		go func(cmd *exec.Cmd) {
			if err := cmd.Wait(); err != nil { // Quit if child exits with abnormal status
				fmt.Printf("EXITING (abnormal child exit: %v)", err)
				os.Exit(1)
			} else if cmd == child {
				hup <- syscall.SIGHUP
			}
		}(cmd)

		child = cmd

		// Prevent another reload from immediately following the previous one
		time.Sleep(500 * time.Millisecond)

		<-hup

		fmt.Println("\nReloading...")
		fmt.Println("")
	}
}

// errorLogger wraps logrus logger so that we can pass it into the handlers
type errorLogger struct {
	log *log.Logger
}

// It implements the required io.Writer interface
func (el *errorLogger) Write(p []byte) (n int, err error) {
	el.log.Errorln(string(p))
	return len(p), nil
}
