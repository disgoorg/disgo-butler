package butler

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo-butler/db"
	"github.com/disgoorg/disgo-butler/mod_mail"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/httpserver"
	"github.com/disgoorg/disgo/oauth2"
	"github.com/disgoorg/disgo/webhook"
	"github.com/disgoorg/log"
	"github.com/disgoorg/utils/paginator"
	"github.com/google/go-github/v44/github"
	"github.com/hhhapz/doc"
	"github.com/hhhapz/doc/godocs"
)

func New(logger log.Logger, version string, config Config) *Butler {
	return &Butler{
		Config:     config,
		Logger:     logger,
		Commands:   map[string]Command{},
		Components: map[string]Component{},
		Webhooks:   map[string]webhook.Client{},
		Paginator:  paginator.NewManager(),
		Version:    version,
	}
}

type Butler struct {
	Client       bot.Client
	OAuth2       oauth2.Client
	Logger       log.Logger
	Mux          *http.ServeMux
	GitHubClient *github.Client
	Paginator    *paginator.Manager
	Commands     map[string]Command
	Components   map[string]Component
	DocClient    *doc.CachedSearcher
	ModMail      *mod_mail.ModMail
	DB           db.DB
	Config       Config
	Webhooks     map[string]webhook.Client
	Version      string
}

func (b *Butler) SetupRoutes(routes http.Handler) {
	b.Mux = http.NewServeMux()
	b.Mux.Handle("/", routes)
}

func (b *Butler) SetupBot() {
	b.ModMail = mod_mail.New(b.Config.ModMail)
	var err error
	if b.Client, err = disgo.New(b.Config.Token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(gateway.IntentGuildMessages|gateway.IntentDirectMessages|gateway.IntentGuildMessageTyping|gateway.IntentDirectMessageTyping|gateway.IntentMessageContent),
			gateway.WithCompress(true),
			gateway.WithPresence(gateway.MessageDataPresenceUpdate{
				Activities: []discord.Activity{
					{
						Name: "loading...",
						Type: discord.ActivityTypeGame,
					},
				},
				Status: discord.OnlineStatusDND,
			}),
		),
		bot.WithCacheConfigOpts(cache.WithCacheFlags(cache.FlagGuilds)),
		bot.WithEventListenerFunc(b.OnReady),
		bot.WithEventListenerFunc(b.OnApplicationCommandInteraction),
		bot.WithEventListenerFunc(b.OnComponentInteraction),
		bot.WithEventListenerFunc(b.OnAutocompleteInteraction),
		bot.WithEventListeners(b.Paginator),
		bot.WithEventListeners(b.ModMail),
		bot.WithHTTPServerConfigOpts(b.Config.Interactions.PublicKey,
			httpserver.WithServeMux(b.Mux),
			httpserver.WithAddress(b.Config.Interactions.Address),
			httpserver.WithURL(b.Config.Interactions.URL),
		),
		bot.WithLogger(b.Logger),
	); err != nil {
		b.Logger.Errorf("Failed to start bot: %s", err)
	}

	b.OAuth2 = oauth2.New(b.Client.ApplicationID(), b.Config.Secret)

	b.GitHubClient = github.NewClient(b.Client.Rest().HTTPClient())
	b.DocClient = doc.WithCache(doc.New(b.Client.Rest().HTTPClient(), godocs.Parser))
	b.Logger.Info("Loading go modules aliases...")
	for _, module := range b.Config.Docs.Aliases {
		_, _ = b.DocClient.Search(context.TODO(), module)
	}
}

func (b *Butler) SetupDB(shouldSyncDBTables bool) {
	var err error
	if b.DB, err = db.SetupDatabase(shouldSyncDBTables, b.Config.Database); err != nil {
		b.Logger.Fatalf("Failed to setup database: %s", err)
	}
}

func (b *Butler) StartAndBlock() {
	if err := b.Client.OpenGateway(context.TODO()); err != nil {
		b.Logger.Errorf("Failed to connect to gateway: %s", err)
	}
	if err := b.Client.OpenHTTPServer(); err != nil {
		b.Logger.Errorf("Failed to start http server: %s", err)
	}

	defer func() {
		b.Logger.Info("Shutting down...")
		b.Client.Close(context.TODO())
		b.DB.Close()
		b.Config.ModMail.Threads = b.ModMail.Close()
		if err := SaveConfig(b.Config); err != nil {
			b.Logger.Errorf("Failed to save config: %s", err)
		}
	}()

	b.Logger.Info("Client is running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s
}

func (b *Butler) OnReady(_ *events.Ready) {
	b.Logger.Infof("Butler ready")
	if err := b.Client.SetPresence(context.TODO(), gateway.MessageDataPresenceUpdate{
		Activities: []discord.Activity{
			{
				Name: "you in DMs",
				Type: discord.ActivityTypeListening,
			},
		},
		Status: discord.OnlineStatusOnline,
	}); err != nil {
		b.Logger.Errorf("Failed to set presence: %s", err)
	}
}
