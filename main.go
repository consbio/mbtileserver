package main

import (
	"strconv"
	"context"
	"fmt"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/evalphobia/logrus_sentry"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/consbio/mbtileserver/handlers"
	"github.com/consbio/mbtileserver/mbtiles"
)

var (
	tilesets    map[string]mbtiles.DB
	startuptime = time.Now()
)

var rootCmd = &cobra.Command{
	Use:   "mbtileserver",
	Short: "Serve tiles from mbtiles files",
	Run: func(cmd *cobra.Command, args []string) {
		if isChild := os.Getenv("MBTS_IS_CHILD"); isChild != "" {
			serve()
		} else {
			supervise()
		}
	},
}

var (
	port        int
	tilePath    string
	certificate string
	privateKey  string
	pathPrefix  string
	domain      string
	secretKey   string
	sentryDSN   string
	verbose     bool
	autotls     bool
	redirect    bool
)

func init() {
	flags := rootCmd.Flags()
	flags.IntVarP(&port, "port", "p", 8000, "Server port.")
	flags.StringVarP(&tilePath, "dir", "d", "./tilesets", "Directory containing mbtiles files.")
	flags.StringVarP(&certificate, "cert", "c", "", "X.509 TLS certificate filename.  If present, will be used to enable SSL on the server.")
	flags.StringVarP(&privateKey, "key", "k", "", "TLS private key")
	flags.StringVar(&pathPrefix, "path", "", "URL root path of this server (if behind a proxy)")
	flags.StringVar(&domain, "domain", "", "Domain name of this server")
	flags.StringVarP(&secretKey, "secret-key", "s", "", "Shared secret key used for HMAC authentication")
	flags.StringVar(&sentryDSN, "dsn", "", "Sentry DSN")
	flags.BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")
	flags.BoolVarP(&autotls, "tls", "t", false, "Auto TLS via Let's Encrypt")
	flags.BoolVarP(&redirect, "redirect", "r", false, "Redirect HTTP to HTTPS")

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

	if env := os.Getenv("TLS_CERT"); env != "" {
		certificate = env
	}

	if env := os.Getenv("TLS_PRIVATE_KEY"); env != "" {
		privateKey = env
	}

	if env := os.Getenv("PATH_PREFIX"); env != "" {
		pathPrefix = env
	}

	if env := os.Getenv("DOMAIN"); env != "" {
		domain = env
	}

	if env := os.Getenv("DSN"); env != "" {
		sentryDSN = env
	}

	if env := os.Getenv("VERBOSE"); env != "" {
		p, err := strconv.ParseBool(env)
		if err != nil {
			log.Fatalln("VERBOSE must be a bool(true/false)")
		}
		verbose = p
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

	if secretKey == "" {
		secretKey = os.Getenv("HMAC_SECRET_KEY")
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

	certExists := len(certificate) > 0
	keyExists := len(privateKey) > 0
	domainExists := len(domain) > 0

	if certExists != keyExists {
		log.Fatalln("Both certificate and private key are required to use SSL")
	}

	if len(pathPrefix) > 0 && !domainExists {
		log.Fatalln("Domain is required if path is provided")
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

	var filenames []string
	err := filepath.Walk(tilePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(strings.ToLower(path), ".mbtiles") {
			filenames = append(filenames, path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Unable to scan tileset directory for mbtiles files\n%v", err)
	}

	if len(filenames) == 0 {
		log.Warnf("No tilesets found in %s!\n", tilePath)
	} else {
		log.Infof("Found %v mbtiles files in %s", len(filenames), tilePath)
	}

	svcSet, err := handlers.NewFromBaseDir(tilePath, secretKey)
	if err != nil {
		log.Errorf("Unable to create service set: %v", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.Pre(middleware.RemoveTrailingSlash())
	if verbose {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	gzip := middleware.Gzip()

	staticPrefix := "/static"
	if pathPrefix != "" {
		staticPrefix = "/" + pathPrefix + staticPrefix
	}
	staticHandler := http.StripPrefix(staticPrefix, handlers.Static())
	e.GET(staticPrefix+"*", echo.WrapHandler(staticHandler), gzip)

	ef := func(err error) {
		log.Errorf("%v", err)
	}
	h := echo.WrapHandler(svcSet.Handler(ef, true))
	e.GET("/*", h)
	a := echo.WrapHandler(svcSet.ArcGISHandler(ef))
	e.GET("/arcgis/rest/services/*", a)

	// Start the server
	fmt.Println("\n--------------------------------------")
	fmt.Println("Use Ctrl-C to exit the server")
	fmt.Println("--------------------------------------")

	// If starting TLS on 443, start server on port 80 too
	if redirect {
		e.Pre(middleware.HTTPSRedirect())
		if port == 443 {
			go func(c *echo.Echo) {
				fmt.Println("HTTP server with redirect started on port 80")
				log.Fatal(e.Start(":80"))
			}(e)
		}
	}

	f := os.NewFile(3, "")
	listener, err := net.FileListener(f)

	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{Handler: e}

	// Listen for SIGHUP (graceful shutdown)
	go func(e *echo.Echo) {
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

			fmt.Printf("HTTPS server started on port %v\n", port)
			log.Fatal(server.ServeTLS(listener, certificate, privateKey))
		}
	case autotls:
		{
			log.Debug("Starting HTTPS using Let's Encrypt")
			e.AutoTLSManager.Cache = autocert.DirCache(".certs")
			e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(domain)

			fmt.Printf("HTTPS server started on port %v\n", port)

			server.TLSConfig.GetCertificate = e.AutoTLSManager.GetCertificate
			server.TLSConfig.NextProtos = append(server.TLSConfig.NextProtos, acme.ALPNProto)
			if !e.DisableHTTP2 {
				server.TLSConfig.NextProtos = append(server.TLSConfig.NextProtos, "h2")
			}

			log.Fatal(server.Serve(listener))
		}
	default:
		{
			fmt.Printf("HTTP server started on port %v\n", port)
			log.Fatal(server.Serve(listener))
		}
	}
}

// The main process forks and manages a sub-process for graceful reloading
func supervise() {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
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

	var child *exec.Cmd = nil
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

	for {
		if shutdown {
			break
		}

		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)

		cmd := createFork()

		go func(cmd *exec.Cmd) {
			if err := cmd.Wait(); err != nil { // Quit if child exits with abnormal status
				os.Exit(1)
			} else if cmd == child {
				hup <- syscall.SIGHUP
			}
		}(cmd)

		if child != nil {
			killFork(child)
		}

		child = cmd

		<-hup

		fmt.Println("\nReloading...")
		fmt.Println("")
	}
}

/*
func notModifiedMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var lastModified time.Time
		id := c.Param("id")
		if _, exists := tilesets[id]; exists {
			lastModified = tilesets[id].TimeStamp()
		} else {
			lastModified = startuptime // startup time of server
		}

		if t, err := time.Parse(http.TimeFormat, c.Request().Header.Get(echo.HeaderIfModifiedSince)); err == nil && lastModified.Before(t.Add(1*time.Second)) {
			c.Response().Header().Del(echo.HeaderContentType)
			c.Response().Header().Del(echo.HeaderContentLength)
			return c.NoContent(http.StatusNotModified)
		}

		c.Response().Header().Set(echo.HeaderLastModified, lastModified.UTC().Format(http.TimeFormat))
		return next(c)
	}
}
*/
