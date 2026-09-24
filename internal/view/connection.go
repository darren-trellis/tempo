package view

import (
	"context"
	"time"

	"github.com/atterpac/jig/theme"
	"github.com/galaxy-io/tempo/internal/temporal"
	"github.com/gdamore/tcell/v2"
)

func (a *App) setConnected(connected bool) {
	if a != nil {
		a.connected = connected
	}
	a.paintConnectionSection(a.loadingText())
	a.refreshCodecStatus()
}

func (a *App) setCodecStatus(icon string, colorFunc func() tcell.Color) {
	if a == nil {
		return
	}
	a.chromeCodecIcon = icon
	a.chromeCodecColor = colorFunc
}

func (a *App) refreshCodecStatus() {
	var endpoint, namespace string
	if a.provider != nil {
		cfg := a.provider.Config()
		endpoint = cfg.CodecEndpoint
		namespace = cfg.Namespace
	}
	if namespace == "" {
		namespace = a.currentNS
	}
	if endpoint == "" {
		a.setCodecStatus("", theme.FgDim)
		return
	}
	if a.chromeCodecIcon == "" {
		a.setCodecStatus(theme.IconCloud, theme.FgDim)
	}
	go func() {
		err := temporal.ProbeCodec(endpoint, namespace)
		a.app.QueueUpdateDraw(func() {
			if err != nil {
				a.setCodecStatus(theme.IconCloud, theme.Error)
				return
			}
			a.setCodecStatus(theme.IconCloud, theme.Success)
		})
	}()
}

// connectionMonitor periodically checks the connection and attempts reconnection if needed.
func (a *App) connectionMonitor() {
	ticker := time.NewTicker(connectionCheckInterval)
	defer ticker.Stop()

	backoff := reconnectInitialBackoff

	for {
		select {
		case <-a.stopMonitor:
			return
		case <-ticker.C:
			// Get provider with lock
			a.mu.RLock()
			provider := a.provider
			a.mu.RUnlock()

			if provider == nil {
				continue
			}

			// Check connection
			ctx, cancel := context.WithTimeout(context.Background(), connectionCheckTimeout)
			err := provider.CheckConnection(ctx)
			cancel()

			if err != nil {
				// Connection lost - update UI
				a.app.QueueUpdateDraw(func() {
					a.setConnected(false)
				})

				// Attempt reconnection with backoff
				a.mu.Lock()
				shouldReconnect := !a.reconnecting
				if shouldReconnect {
					a.reconnecting = true
				}
				a.mu.Unlock()

				if shouldReconnect {
					go a.attemptReconnect(backoff)
					backoff = backoff * 2
					if backoff > reconnectMaxBackoff {
						backoff = reconnectMaxBackoff
					}
				}
			} else {
				// Connection is good - reset backoff
				backoff = reconnectInitialBackoff
				a.mu.Lock()
				a.reconnecting = false
				a.mu.Unlock()

				// Ensure UI shows connected
				a.app.QueueUpdateDraw(func() {
					a.setConnected(true)
				})
			}
		}
	}
}

// attemptReconnect tries to reconnect to the Temporal server.
func (a *App) attemptReconnect(backoff time.Duration) {
	select {
	case <-a.stopMonitor:
		return
	case <-time.After(backoff):
	}

	// Get provider with lock
	a.mu.RLock()
	provider := a.provider
	a.mu.RUnlock()

	if provider == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := provider.Reconnect(ctx)
	cancel()

	// Update reconnecting state before QueueUpdateDraw to avoid deadlock
	if err == nil {
		a.mu.Lock()
		a.reconnecting = false
		a.mu.Unlock()
	}

	a.app.QueueUpdateDraw(func() {
		if err == nil {
			a.setConnected(true)
			a.refreshNamespaceCatalog()
		}
	})
}
