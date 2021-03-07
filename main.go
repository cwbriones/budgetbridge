package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"budgetbridge/ynab"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func getBudgetID(ctx context.Context, ynabClient *ynab.Client, config Config) (string, error) {
	if config.BudgetID != nil {
		log.Debug().Msg("using pre-configured budget_id")
		return *config.BudgetID, nil
	}
	log.Debug().Msg("no budget_id configured, fetching")
	res, err := ynabClient.Budgets(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch budget_id: %s", err)
	}
	if len(res.Budgets) == 1 {
		return res.Budgets[0].Id, nil
	}
	if res.DefaultBudget != nil {
		return res.DefaultBudget.Id, nil
	}
	return "", fmt.Errorf("no default budget available")
}

func newYNABClient(ctx context.Context, accessToken string) *ynab.Client {
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	}))
	return ynab.NewClient(httpClient)
}

func initLogging() func() error {
	level := zerolog.InfoLevel
	if envlevel, ok := getLogLevelEnv(); ok {
		level = envlevel
	}
	zerolog.SetGlobalLevel(level)
	w := bufio.NewWriter(os.Stderr)
	log.Logger = zerolog.
		New(w).
		With().
		Timestamp().
		Logger()

	return w.Flush
}

func getLogLevelEnv() (zerolog.Level, bool) {
	var level zerolog.Level

	lvlstr := os.Getenv("LOG")
	if lvlstr == "" {
		return level, false
	}
	if level, err := zerolog.ParseLevel(lvlstr); err == nil {
		return level, true
	}
	return level, false
}

type dateFlag struct {
	time   time.Time
	layout string
}

func (f *dateFlag) Set(value string) error {
	var t time.Time
	var err error
	if t, err = time.Parse(f.layout, value); err != nil {
		return err
	}
	f.time = t
	return nil
}

func (f *dateFlag) String() string {
	return f.time.Format(f.layout)
}

func main() {
	configPath := flag.String("config", "config.json", "the path of your config.json file")
	dryRun := flag.Bool("dry", false, "emit the transactions but do not create them.")

	lastUpdateHint := dateFlag{
		layout: "2006-01-02",
	}
	flag.Var(&lastUpdateHint, "since", "how far to look back for transactions.")

	flag.Parse()
	flush := initLogging()
	defer flush()
	defer func() {
		if v := recover(); v != nil {
			var event *zerolog.Event
			if e, ok := v.(error); ok {
				event = log.Err(e)
			} else if e, ok := v.(string); ok {
				event = log.Error().Str("error", e)
			}
			stack := strings.Split(string(debug.Stack()), "\n")
			event.
				Str("type", fmt.Sprintf("%T\n", v)).
				Strs("stack", stack).
				Msg("exiting due to panic")
			os.Exit(1)
		}
	}()

	var config Config
	err := config.Providers.SetRegistry(map[string]NewProvider{
		"splitwise": &SplitwiseOptions{},
	})
	check(err)

	err = config.load(*configPath)
	check(err)

	ctx := context.Background()

	ynabClient := newYNABClient(ctx, config.AccessToken)

	if config.Cache.CreateMissingDir {
		err = os.MkdirAll(config.Cache.Dir, os.ModePerm)
		check(err)
	}
	budgetID, err := getBudgetID(ctx, ynabClient, config)
	check(err)
	categoriesCache := CategoriesCache{
		client:   ynabClient,
		budgetID: budgetID,
		enabled:  true,
		path:     filepath.Join(config.Cache.Dir, "categories"),
	}

	categories, err := categoriesCache.Categories(ctx)
	check(err)

	providers := config.Providers.initAll(ctx)
	if len(providers) == 0 {
		log.Warn().Msg("no providers are configured")
		return
	}

	bridge := BudgetBridge{
		budgetID,
		config.LookBackDays,
		ynabClient,
		providers,
		categories,
		*dryRun,
	}
	err = bridge.ImportAll(ctx, config)
	check(err)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
