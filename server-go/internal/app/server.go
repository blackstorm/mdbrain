package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/http/handlers"
	httpmw "mdbrain.dev/internal/http/middleware"
	dbinfra "mdbrain.dev/internal/infra/db"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/infra/storage"
	"mdbrain.dev/internal/templatex"
)

type ServerSet struct {
	Config        *config.Config
	AppServer     *http.Server
	ConsoleServer *http.Server
	close         func() error
}

func Build(ctx context.Context, projectRoot string) (*ServerSet, error) {
	cfg, err := config.Load(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	db, err := dbinfra.OpenSQLite(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cleanupDB := true
	defer func() {
		if cleanupDB {
			_ = dbinfra.Close(db)
		}
	}()
	repo := repository.New(db)
	objectStore, err := storage.NewObjectStore(cfg)
	if err != nil {
		return nil, err
	}
	renderer, err := templatex.New(cfg.TemplateRoot)
	if err != nil {
		return nil, err
	}
	sessions := httpmw.NewSessionManager(cfg.SessionHashKey(), cfg.Production())

	appHandler := handlers.NewAppHandler(cfg, repo, objectStore, renderer)
	internalHandler := handlers.NewInternalHandler(cfg, repo)
	syncHandler := handlers.NewSyncHandler(repo, objectStore)
	consoleAuth := handlers.NewConsoleAuthHandler(repo, renderer, sessions)
	consoleVaults := handlers.NewConsoleVaultHandler(repo, objectStore, renderer)
	consoleCommon := handlers.NewConsoleCommonHandler(repo, objectStore)
	consoleLogo := handlers.NewConsoleLogoHandler(repo, objectStore)

	appEcho := echo.New()
	appEcho.GET("/favicon.ico", appHandler.ServeFavicon)
	appEcho.GET("/storage/*", appHandler.ServeAsset)
	appEcho.Static("/publics/app", filepath.Join(cfg.PublicRoot, "app"))
	appEcho.Static("/publics/shared", filepath.Join(cfg.PublicRoot, "shared"))
	appEcho.GET("/", appHandler.GetNote)
	appEcho.GET("/*", appHandler.GetNote)

	consoleEcho := echo.New()
	consoleEcho.Use(httpmw.ObsidianCORSMiddleware())
	consoleEcho.Use(httpmw.SessionMiddleware(sessions))
	consoleEcho.Use(httpmw.ConsoleCSRFMiddleware())
	consoleEcho.Use(httpmw.ConsoleInitCheckMiddleware(repo))
	consoleEcho.Use(httpmw.ConsoleNoIndexMiddleware())
	consoleEcho.Static("/publics/console", filepath.Join(cfg.PublicRoot, "console"))
	consoleEcho.Static("/publics/shared", filepath.Join(cfg.PublicRoot, "shared"))
	consoleEcho.GET("/", func(c *echo.Context) error { return c.Redirect(http.StatusFound, "/console") })
	consoleEcho.GET("/robots.txt", internalHandler.Robots)
	consoleEcho.OPTIONS("/obsidian/sync/changes", syncHandler.SyncChanges)
	consoleEcho.POST("/obsidian/sync/changes", syncHandler.SyncChanges)
	consoleEcho.OPTIONS("/obsidian/sync/notes/:id", syncHandler.SyncNote)
	consoleEcho.POST("/obsidian/sync/notes/:id", syncHandler.SyncNote)
	consoleEcho.OPTIONS("/obsidian/sync/assets/:id", syncHandler.SyncAsset)
	consoleEcho.POST("/obsidian/sync/assets/:id", syncHandler.SyncAsset)
	consoleEcho.GET("/obsidian/vault/info", syncHandler.VaultInfo)
	consoleEcho.GET("/console/health", internalHandler.Health)
	if cfg.OnDemandTLSEnabled {
		consoleEcho.GET("/console/domain-check", internalHandler.DomainCheck)
	}
	consoleEcho.GET("/console/login", consoleAuth.LoginPage)
	consoleEcho.POST("/console/login", consoleAuth.Login)
	consoleEcho.GET("/console/init", consoleAuth.InitPage)
	consoleEcho.POST("/console/init", consoleAuth.InitConsole)
	consoleEcho.POST("/console/logout", consoleAuth.Logout, httpmw.ConsoleAuthMiddleware())
	consoleEcho.PUT("/console/user/password", consoleAuth.ChangePassword, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console", consoleVaults.ConsoleHome, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console/vaults", consoleVaults.ListVaults, httpmw.ConsoleAuthMiddleware())
	consoleEcho.POST("/console/vaults", consoleVaults.CreateVault, httpmw.ConsoleAuthMiddleware())
	consoleEcho.PUT("/console/vaults/:id", consoleVaults.UpdateVault, httpmw.ConsoleAuthMiddleware())
	consoleEcho.DELETE("/console/vaults/:id", consoleVaults.DeleteVault, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console/vaults/:id/notes", consoleVaults.SearchVaultNotes, httpmw.ConsoleAuthMiddleware())
	consoleEcho.PUT("/console/vaults/:id/root-note", consoleVaults.UpdateVaultRootNote, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console/vaults/:id/root-note-selector", consoleVaults.GetRootNoteSelector, httpmw.ConsoleAuthMiddleware())
	consoleEcho.POST("/console/vaults/:id/renew-sync-key", consoleVaults.RenewVaultSyncKey, httpmw.ConsoleAuthMiddleware())
	consoleEcho.PUT("/console/vaults/:id/custom-head-html", consoleVaults.UpdateCustomHeadHTML, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console/vaults/:id/logo", consoleLogo.ServeVaultLogo, httpmw.ConsoleAuthMiddleware())
	consoleEcho.POST("/console/vaults/:id/logo", consoleLogo.UploadVaultLogo, httpmw.ConsoleAuthMiddleware())
	consoleEcho.DELETE("/console/vaults/:id/logo", consoleLogo.DeleteVaultLogo, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console/vaults/:id/favicon", consoleLogo.ServeVaultFavicon, httpmw.ConsoleAuthMiddleware())
	consoleEcho.GET("/console/storage/:id/*", consoleCommon.ServeConsoleAsset, httpmw.ConsoleAuthMiddleware())

	servers := &ServerSet{
		Config: cfg,
		AppServer: &http.Server{
			Addr:    cfg.AppHost + ":" + intToString(cfg.AppPort),
			Handler: appEcho,
		},
		ConsoleServer: &http.Server{
			Addr:    cfg.ConsoleHost + ":" + intToString(cfg.ConsolePort),
			Handler: consoleEcho,
		},
		close: func() error {
			return dbinfra.Close(db)
		},
	}
	cleanupDB = false
	return servers, nil
}

func (s *ServerSet) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	go func() { errCh <- s.AppServer.ListenAndServe() }()
	go func() { errCh <- s.ConsoleServer.ListenAndServe() }()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	select {
	case err := <-errCh:
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case <-signalCh:
		return s.Shutdown(context.Background())
	}
}

func (s *ServerSet) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.AppServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
		return err
	}
	if err := s.ConsoleServer.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
		return err
	}
	if s.close != nil {
		if err := s.close(); err != nil {
			return err
		}
		s.close = nil
	}
	return nil
}

func intToString(v int) string {
	return strconv.Itoa(v)
}
