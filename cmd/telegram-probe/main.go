// telegram-probe is an operator-run integration spike, not a production worker.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"golang.org/x/term"
)

const (
	historyLimit = 25
	maxBytes int64 = 32 << 20
)

var (
	errLimit = errors.New("probe history limit reached")
	errSize = errors.New("probe download size limit reached")
)

func main() {
	state := flag.String("state-dir", "", "dedicated private V2 test directory (required)")
	messageID := flag.Int("message-id", 0, "explicit Saved Messages media ID to download (0 = list only)")
	flag.Parse()
	if err := run(*state, *messageID); err != nil {
		// Keep credential values, RPC payloads and QR tokens out of logs.
		fmt.Fprintf(os.Stderr, "probe failed (%T); inspect the stage above and verify settings\n", err)
		os.Exit(1)
	}
}

func run(state string, messageID int) error {
	appID, err := strconv.Atoi(os.Getenv("TG_API_ID"))
	appHash := strings.TrimSpace(os.Getenv("TG_API_HASH"))
	if err != nil || appID <= 0 || appHash == "" || state == "" || messageID < 0 {
		return errors.New("TG_API_ID, TG_API_HASH and -state-dir required; message ID must be nonnegative")
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(state, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(state)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("state directory must be private and not a symlink")
	}
	lockPath := filepath.Join(state, "probe.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_ = lock.Close()
	defer os.Remove(lockPath)

	sessionPath := filepath.Join(state, "probe.session.json")
	if info, err := os.Lstat(sessionPath); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("session must be a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(&dispatcher)
	client := telegram.NewClient(appID, appHash, telegram.Options{
		UpdateHandler: dispatcher,
		SessionStorage: &session.FileStorage{Path: sessionPath},
	})

	fmt.Println("Connecting with isolated V2 session...")
	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			if err := authenticate(ctx, client, loggedIn, state); err != nil {
				return err
			}
		}
		user, err := client.Self(ctx)
		if err != nil {
			return err
		}
		if user.Bot {
			return errors.New("probe requires a user account")
		}
		fmt.Println("User authorized; reading up to 25 Saved Messages (no message text logged).")
		return history(ctx, client, state, messageID)
	})
}

func authenticate(ctx context.Context, client *telegram.Client, loggedIn qrlogin.LoggedIn, state string) error {
	var currentQR string
	defer func() {
		if currentQR != "" {
			_ = os.Remove(currentQR)
		}
	}()
	_, err := client.QR().Auth(ctx, loggedIn, func(_ context.Context, token qrlogin.Token) error {
		img, err := token.Image(1)
		if err != nil {
			return err
		}
		f, err := os.CreateTemp(state, "login-*.png")
		if err != nil {
			return err
		}
		if currentQR != "" {
			_ = os.Remove(currentQR)
		}
		currentQR = f.Name()
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Printf("Open the private QR image locally: %s (expires %s)\n", currentQR, token.Expires().Format(time.RFC3339))
		fmt.Println("Telegram > Settings > Devices > Link Desktop Device")
		return nil
	})
	if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("2FA requires an interactive terminal")
		}
		fmt.Fprint(os.Stderr, "Telegram 2FA password: ")
		password, readErr := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if readErr != nil {
			return readErr
		}
		defer clear(password)
		_, err = client.Auth().Password(ctx, string(password))
	}
	return err
}

func history(ctx context.Context, client *telegram.Client, state string, messageID int) error {
	count, found := 0, false
	err := query.Messages(client.API()).GetHistory(&tg.InputPeerSelf{}).
		ForEach(ctx, func(ctx context.Context, elem messages.Elem) error {
			if count >= historyLimit {
				return errLimit
			}
			count++
			msg, ok := elem.Msg.(*tg.Message)
			if !ok {
				return nil
			}
			file, media := elem.File()
			fmt.Printf("message_id=%d media=%t\n", msg.ID, media)
			if messageID != 0 && msg.ID == messageID && media {
				if err := download(ctx, client, state, msg.ID, file.Location); err != nil {
					return err
				}
				found = true
				return errLimit
			}
			return nil
		})
	if err != nil && !errors.Is(err, errLimit) {
		return err
	}
	if messageID != 0 && !found {
		return errors.New("requested media not present in first 25 Saved Messages")
	}
	return nil
}

type boundedWriter struct {
	w io.Writer
	remaining int64
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errSize
	}
	n, err := w.w.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func download(ctx context.Context, client *telegram.Client, state string, id int, location tg.InputFileLocationClass) error {
	fmt.Printf("Downloading selected message %d (32 MiB maximum)...\n", id)
	f, err := os.CreateTemp(state, "media-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	limited := &boundedWriter{w: f, remaining: maxBytes}
	if _, err := client.Download(location).Stream(ctx, limited); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Link fails rather than overwriting an existing successful probe file.
	dest := filepath.Join(state, fmt.Sprintf("message-%d.bin", id))
	if err := os.Link(f.Name(), dest); err != nil {
		return err
	}
	fmt.Printf("Download completed: %d bytes; %s\n", maxBytes-limited.remaining, dest)
	return nil
}
