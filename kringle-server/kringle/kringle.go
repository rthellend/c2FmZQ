package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-shellwords" // shellwords
	"github.com/urfave/cli/v2"       // cli
	"golang.org/x/term"

	"kringle-server/client"
	"kringle-server/crypto"
	"kringle-server/log"
	"kringle-server/secure"
)

type kringle struct {
	client *client.Client
	cli    *cli.App
	term   *term.Terminal

	// flags
	flagDataDir        string
	flagLogLevel       int
	flagPassphraseFile string
	flagAPIServer      string
}

func makeKringle() *kringle {
	var app kringle
	app.cli = &cli.App{
		Name:     "kringle",
		Usage:    "kringle client.",
		HideHelp: true,
		CommandNotFound: func(ctx *cli.Context, cmd string) {
			fmt.Fprintf(app.cli.Writer, "Unknown command %q. Try \"help\"\n", cmd)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "data-dir",
				Aliases:     []string{"d"},
				Value:       "",
				Usage:       "Save the data in `DIR`",
				Required:    true,
				EnvVars:     []string{"KRINGLE_DATADIR"},
				TakesFile:   true,
				Destination: &app.flagDataDir,
			},
			&cli.IntFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Value:       2,
				DefaultText: "2 (info)",
				Usage:       "The level of logging verbosity: 1:Error 2:Info 3:Debug",
				Destination: &app.flagLogLevel,
			},
			&cli.StringFlag{
				Name:        "passphrase-file",
				Value:       "",
				Usage:       "Read the database passphrase from `FILE`.",
				EnvVars:     []string{"KRINGLE_PASSPHRASE_FILE"},
				Destination: &app.flagPassphraseFile,
			},
			&cli.StringFlag{
				Name:        "server",
				Value:       "",
				Usage:       "The API server base URL.",
				Destination: &app.flagAPIServer,
			},
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:     "shell",
				Usage:    "Run in shell mode.",
				Action:   app.shell,
				Category: "Mode",
			},
			&cli.Command{
				Name:      "create-account",
				Usage:     "Create an account.",
				ArgsUsage: "<email>",
				Action:    app.createAccount,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "login",
				Usage:     "Login to an account.",
				ArgsUsage: "<email>",
				Action:    app.login,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "logout",
				Usage:     "Logout.",
				ArgsUsage: " ",
				Action:    app.logout,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "updates",
				Aliases:   []string{"up", "update"},
				Usage:     "Pull metadata updates.",
				ArgsUsage: " ",
				Action:    app.updates,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "download",
				Aliases:   []string{"pull"},
				Usage:     "Download files that aren't already downloaded.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    app.pullFiles,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "upload",
				Aliases:   []string{"push", "sync"},
				Usage:     "Upload files that haven't been uploaded yet.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    app.pushFiles,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "free",
				Usage:     "Remove local files that are backed up.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    app.freeFiles,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "hide",
				Usage:     "Hide albums.",
				ArgsUsage: `["glob"] ...`,
				Action:    app.hideAlbums,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "unhide",
				Usage:     "Unhide albums.",
				ArgsUsage: "[name] ...",
				Action:    app.unhideAlbums,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "list",
				Aliases:   []string{"ls"},
				Usage:     "List albums and files.",
				ArgsUsage: `["glob"] ... (default "*")`,
				Action:    app.listFiles,
				Category:  "Files",
			},
			&cli.Command{
				Name:      "export",
				Usage:     "Decrypt and export files.",
				ArgsUsage: `"<glob>" ... <output directory>`,
				Action:    app.exportFiles,
				Category:  "Import/Export",
			},
			&cli.Command{
				Name:      "import",
				Usage:     "Encrypt and import files.",
				ArgsUsage: `"<glob>" ... <album>`,
				Action:    app.importFiles,
				Category:  "Import/Export",
			},
		},
	}
	sort.Sort(cli.CommandsByName(app.cli.Commands))

	return &app
}

func (k *kringle) initClient(ctx *cli.Context, update bool) error {
	if k.client == nil {
		log.Level = k.flagLogLevel
		var pp string
		var err error
		if pp, err = k.passphrase(ctx); err != nil {
			return err
		}

		mkFile := filepath.Join(k.flagDataDir, "master.key")
		masterKey, err := crypto.ReadMasterKey(pp, mkFile)
		if errors.Is(err, os.ErrNotExist) {
			if masterKey, err = crypto.CreateMasterKey(); err != nil {
				log.Fatal("Failed to create master key")
			}
			err = masterKey.Save(pp, mkFile)
		}
		if err != nil {
			log.Fatalf("Failed to decrypt master key: %v", err)
		}
		storage := secure.NewStorage(k.flagDataDir, &masterKey.EncryptionKey)

		c, err := client.Load(storage)
		if err != nil {
			c, err = client.Create(storage)
		}
		if k.flagAPIServer != "" {
			c.ServerBaseURL = k.flagAPIServer
		}
		k.client = c
	}
	if update {
		if err := k.client.GetUpdates(true); err != nil {
			return err
		}
	}
	return nil
}

func (k *kringle) shell(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("not a terminal")
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	t := term.NewTerminal(screen, "kringle> ")
	/*
		t.AutoCompleteCallback = func(line string, pos int, key rune) (newLine string, newPos int, ok bool) {
			if key == '\t' {
				// Do something
			}
			return
		}
	*/
	k.cli.Writer = t
	k.client.SetWriter(t)
	k.term = t

	p := shellwords.NewParser()

	for {
		t.SetPrompt(string(t.Escape.Green) + "kringle> " + string(t.Escape.Reset))
		line, err := t.ReadLine()
		if err == io.EOF {
			return nil
		}
		line = strings.TrimSpace(line)
		args, err := p.Parse(line)
		if err != nil {
			fmt.Fprintf(t, "p.Parse: %v\n", err)
		}
		if len(args) == 0 {
			continue
		}
		switch args[0] {
		case "exit":
			return nil
		case "help":
			if len(args) > 1 {
				t.Write(t.Escape.Blue)
				cli.ShowCommandHelp(ctx, args[1])
				t.Write(t.Escape.Reset)
			} else {
				t.Write(t.Escape.Blue)
				cli.ShowCommandHelp(ctx, "")
				t.Write(t.Escape.Reset)
			}
		case "shell":
			fmt.Fprintf(t, "%sWe Need To Go Deeper%s\n", t.Escape.Red, t.Escape.Reset)
			fallthrough
		default:
			args = append([]string{"kringle"}, args...)
			if err := k.cli.Run(args); err != nil {
				fmt.Fprintf(t, "%s%v%s\n", t.Escape.Red, err, t.Escape.Reset)
			}
		}
	}
}

func (k *kringle) passphrase(ctx *cli.Context) (string, error) {
	if f := k.flagPassphraseFile; f != "" {
		p, err := os.ReadFile(f)
		if err != nil {
			return "", cli.Exit(err, 1)
		}
		return string(p), nil
	}
	return k.promptPass("Enter database passphrase: ")
}

func (k *kringle) promptPass(msg string) (string, error) {
	if k.term != nil {
		return k.term.ReadPassword(string(k.term.Escape.Green) + msg + string(k.term.Escape.Reset))
	}
	fmt.Print(msg)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return string(b), err
}

func (k *kringle) prompt(msg string) (reply string, err error) {
	if k.term != nil {
		k.term.SetPrompt(string(k.term.Escape.Green) + msg + string(k.term.Escape.Reset))
		return k.term.ReadLine()
	}
	fmt.Print(msg)
	_, err = fmt.Scanln(&reply)
	return
}

func (k *kringle) createAccount(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if k.client.ServerBaseURL == "" {
		var err error
		if k.client.ServerBaseURL, err = k.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = k.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}

	password, err := k.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return k.client.CreateAccount(email, password)
}

func (k *kringle) login(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if k.client.ServerBaseURL == "" {
		var err error
		if k.client.ServerBaseURL, err = k.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = k.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}

	password, err := k.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	if err := k.client.Login(email, password); err != nil {
		return err
	}
	return k.client.GetUpdates(true)
}

func (k *kringle) logout(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	return k.client.Logout()
}

func (k *kringle) updates(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	return k.client.GetUpdates(false)
}

func (k *kringle) pullFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Pull(patterns)
}

func (k *kringle) pushFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Push(patterns)
}

func (k *kringle) freeFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Free(patterns)
}

func (k *kringle) hideAlbums(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Hide(patterns, true)
}

func (k *kringle) unhideAlbums(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Hide(patterns, false)
}

func (k *kringle) listFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.ListFiles(patterns)
}

func (k *kringle) exportFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	return k.client.ExportFiles(patterns, dir)
}

func (k *kringle) importFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	return k.client.ImportFiles(patterns, dir)
}