package main

import (
	"fmt"
	"strconv"

	"golang.org/x/crypto/acme/autocert"

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
		serve()
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
			log.Fatal(e.StartTLS(fmt.Sprintf(":%v", port), certificate, privateKey))
		}
	case autotls:
		{
			log.Debug("Starting HTTPS using Let's Encrypt")
			e.AutoTLSManager.Cache = autocert.DirCache(".certs")
			e.AutoTLSManager.HostPolicy = autocert.HostWhitelist(domain)
			fmt.Printf("HTTPS server started on port %v\n", port)
			log.Fatal(e.StartAutoTLS(fmt.Sprintf(":%v", port)))
		}
	default:
		{
			fmt.Printf("HTTP server started on port %v\n", port)
			log.Fatal(e.Start(fmt.Sprintf(":%v", port)))
		}
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
