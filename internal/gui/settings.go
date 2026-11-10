package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/settings"
	"github.com/missuo/xdl/internal/version"
)

func (a *App) loadToken() {
	token, err := backend.GetStoredAuthToken("public")
	a.win.Update(func() {
		if err != nil {
			a.toast("Keychain: " + err.Error())
			return
		}
		a.token, a.tokenSaved = token, token != ""
	})
}

func (a *App) saveToken() {
	token := strings.TrimSpace(a.token)
	go func() {
		err := backend.SetStoredAuthToken("public", token)
		a.win.Update(func() {
			if err != nil {
				a.toast("Saving the token failed: " + err.Error())
				return
			}
			a.tokenSaved = token != ""
			if token == "" {
				a.toast("Removed the auth token")
			} else {
				a.toast("Saved the auth token to the Keychain")
			}
		})
	}()
}

func (a *App) chooseFolder() {
	start := a.downloadDir()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "Download Folder", ButtonLabel: "Choose",
			DefaultPath: start, Directory: true, CreateDirectories: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() {
			a.cfg.DownloadPath = paths[0]
			a.saveSettings()
		})
	}()
}

var settingsTabs = []string{"General", "Downloads", "Dependencies", "Integrations"}

func (a *App) settingsPage(c *ui.Context) {
	pageHeader(c, "Settings", "Shared with the CLI and `xdl serve`: "+settings.Path(), nil)
	a.historyDialog(c)
	if ui.Segmented(c, &a.settingsTab, settingsTabs...).Label("Settings section").Changed() && a.settingsTab == 2 && !a.depChecked {
		a.checkDeps()
	}
	ui.Scroll(c).Grow(1).Basis(0).Gap(16).Padding(0, 4, 8, 0).Children(func() {
		switch a.settingsTab {
		case 0:
			a.generalSettings(c)
		case 1:
			a.downloadSettings(c)
		case 2:
			a.depsSection(c)
		case 3:
			a.integrations(c)
		}
	})
}

func (a *App) generalSettings(c *ui.Context) {
	card(c, "X Account", func() {
		ui.Form(c, func() {
			ui.Field(c, "auth_token", func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.token).Password().Placeholder("40-character auth_token cookie").Grow(1)
					if ui.PrimaryButton(c, "Save").Clicked() {
						a.saveToken()
					}
				})
			}).Description(tokenHint(a.tokenSaved, a.token))
		})
	})
	card(c, "Fetching", func() {
		ui.Form(c, func() {
			ui.Field(c, "Video quality", func() {
				if ui.Select(c, &a.cfg.VideoQuality, settings.VideoQualities).Width(140).Changed() {
					a.saveSettings()
				}
			})
			ui.Field(c, "Image size", func() {
				if ui.Select(c, &a.cfg.ImageSize, settings.ImageSizes).Width(140).Changed() {
					a.saveSettings()
				}
			})
			ui.Field(c, "Seconds per page", func() {
				if ui.NumberInput(c, &a.timeout, 10, 600, 10).Width(110).Changed() {
					a.saveSettings()
				}
			}).Description("A page that takes longer stops the fetch; what came before is kept.")
			ui.Field(c, "Incremental refetch", func() {
				if ui.Switch(c, &a.cfg.Incremental).Changed() {
					a.saveSettings()
				}
			}).Description("Refetching a complete account stops where its saved copy begins.")
			ui.Field(c, "Proxy", func() {
				if ui.TextInput(c, &a.cfg.Proxy).Placeholder("http://127.0.0.1:7890 — empty uses the system's").Changed() {
					a.saveSettings()
				}
			})
		})
	})
	a.updateSettings(c)
	card(c, "Notifications", func() {
		ui.Form(c, func() {
			ui.Field(c, "Notify when a job finishes", func() {
				if ui.Switch(c, &a.cfg.Notify).Changed() {
					a.saveSettings()
				}
			}).Description("Jobs from the browser, the CLI, agents and subscriptions say when they are done; the app's own only while XDL is in the background.")
		})
	})
	ui.Row(c).Children(func() {
		ui.Spacer(c)
		if ui.Button(c, "Restore Defaults").Clicked() {
			a.cfg = settings.Defaults()
			a.loadNumbers()
			a.saveSettings()
		}
	})
}

func (a *App) downloadSettings(c *ui.Context) {
	card(c, "Downloads", func() {
		ui.Form(c, func() {
			ui.Field(c, "Folder", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.downloadDir()).FontSize(12.5).SingleLine().Grow(1).Selectable()
					if ui.Button(c, "Choose…").Clicked() {
						a.chooseFolder()
					}
					if ui.Button(c, "Open").Clicked() {
						a.openPath(a.downloadDir())
					}
				})
			})
			ui.Field(c, "Parallel downloads", func() {
				if ui.NumberInput(c, &a.concurrent, 1, 50, 1).Width(110).Changed() {
					a.saveSettings()
				}
			})
			ui.Field(c, "Retries per file", func() {
				if ui.NumberInput(c, &a.retries, 1, 5, 1).Width(110).Changed() {
					a.saveSettings()
				}
			})
			ui.Field(c, "Skip files already on disk", func() {
				if ui.Switch(c, &a.cfg.SkipExisting).Changed() {
					a.saveSettings()
				}
			})
			ui.Field(c, "Delete incomplete files", func() {
				if ui.Switch(c, &a.cfg.DeleteIncompleteFiles).Changed() {
					a.saveSettings()
				}
			})
		})
	})
	card(c, "Download History", func() {
		ui.Form(c, func() {
			ui.Field(c, "Skip what was downloaded before", func() {
				if ui.Switch(c, &a.cfg.DownloadHistory).Changed() {
					a.saveSettings()
				}
			}).Description(a.historyHint())
		})
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, "Clear History…").Disabled(a.historyCount == 0).Clicked() {
				a.confirmHistory = true
			}
		})
	})
	card(c, "File Names", func() {
		ui.Form(c, func() {
			ui.Field(c, "File name", func() {
				if ui.TextInput(c, &a.cfg.FilenameTemplate).Font("monospace").Changed() {
					a.saveSettings()
				}
			}).Description("{handle} {name} {date} {tweet_id} {index} {media_id} {type}")
			ui.Field(c, "Folder", func() {
				if ui.TextInput(c, &a.cfg.FolderTemplate).Font("monospace").Changed() {
					a.saveSettings()
				}
			}).Description("{handle} {name} {date}")
		})
	})
	card(c, "GIFs", func() {
		ui.Form(c, func() {
			ui.Field(c, "Convert to .gif after download", func() {
				if ui.Switch(c, &a.cfg.AutoConvertGIFs).Disabled(!backend.IsFFmpegInstalled()).Changed() {
					a.saveSettings()
				}
			}).Description("X serves GIFs as MP4 videos; converting needs FFmpeg.")
			ui.Field(c, "Quality", func() {
				if ui.Select(c, &a.cfg.GIFQuality, settings.GIFQualities).Width(140).Changed() {
					a.saveSettings()
				}
			})
			ui.Field(c, "Resolution", func() {
				if ui.Select(c, &a.cfg.GIFResolution, settings.GIFResolutions).Width(140).Changed() {
					a.saveSettings()
				}
			})
		})
	})
}

func (a *App) integrations(c *ui.Context) {
	t := c.Theme()
	exe, _ := os.Executable()
	status := "Listening on http://" + a.srv.Addr()
	col := t.TextMuted
	if a.apiErr != "" {
		status, col = a.apiErr, t.Danger
	}
	integration := func(ic *ui.SVG, title, body string, actions func()) {
		card(c, "", func() {
			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Icon(c, ic).FontSize(22).TextColor(t.Accent)
				ui.Column(c).Grow(1).Gap(6).Children(func() {
					ui.Text(c, title).FontSize(13.5).Bold()
					ui.Text(c, body).FontSize(12).TextColor(t.TextMuted)
					ui.Row(c).Gap(8).Wrap().Children(actions)
				})
			})
		})
	}
	ui.Text(c, status).FontSize(12).TextColor(col).Selectable()

	integration(iconExtension, "Chrome extension",
		"Adds an XDL button to Chrome: on an X profile it sends the account here, which fetches and downloads it with these settings.",
		func() {
			ext := extensionDir()
			if ui.Button(c, "Show Extension Folder").Disabled(ext == "").
				Tooltip("In chrome://extensions turn on Developer mode, then Load unpacked this folder").Clicked() {
				a.openPath(ext)
			}
			if ui.Button(c, "Copy chrome://extensions").Clicked() {
				c.WriteClipboard("chrome://extensions")
				a.toast("Copied chrome://extensions — paste it in Chrome's address bar")
			}
		})

	integration(iconLogs, "Command line",
		"This app's executable is also the `xdl` command: `xdl download <user>`, `xdl sub add <user> 1d`, `xdl jobs`… Jobs it starts join this queue.",
		func() {
			cmd := `sudo ln -sf "` + exe + `" /usr/local/bin/xdl`
			if runtime.GOOS == "windows" {
				// XDL.exe has no console: the release's xdl.exe is the CLI.
				cmd = `xdl.exe help`
			}
			if ui.Button(c, "Copy Install Command").Tooltip(cmd).Clicked() {
				c.WriteClipboard(cmd)
				a.toast("Copied — paste it in a terminal")
			}
		})

	mcpConfig := `{
  "mcpServers": {
    "xdl": { "command": "` + strings.ReplaceAll(exe, `\`, `\\`) + `", "args": ["mcp"] }
  }
}`
	integration(iconAgent, "AI agents (MCP)",
		"Lets agents such as Claude download accounts, follow jobs, search the library and manage subscriptions. "+
			"Add the server to the agent's MCP settings; it talks to this app while it runs.",
		func() {
			if ui.Button(c, "Copy MCP Config").Tooltip(mcpConfig).Clicked() {
				c.WriteClipboard(mcpConfig)
				a.toast("Copied the MCP server config")
			}
			if ui.Button(c, "Copy Claude Code Command").Clicked() {
				c.WriteClipboard(`claude mcp add xdl -- "` + exe + `" mcp`)
				a.toast("Copied — run it in a terminal")
			}
			if ui.Button(c, "Copy HTTP Endpoint").Tooltip("Streamable HTTP; send the bearer token from api.json").Clicked() {
				c.WriteClipboard("http://" + a.srv.Addr() + "/mcp")
				a.toast("Copied the MCP endpoint")
			}
		})
}

func (a *App) historyHint() string {
	return fmt.Sprintf("Remembers each photo and video by its X media ID (%d so far), so a job skips it without a request — even after you renamed, moved or deleted the file, or changed the name templates.", a.historyCount)
}

func (a *App) loadHistoryCount() {
	n, err := backend.DownloadHistoryCount()
	if err != nil {
		return
	}
	a.win.Update(func() { a.historyCount = n })
}

func (a *App) historyDialog(c *ui.Context) {
	if ui.AlertDialog(c, &a.confirmHistory, "Clear the download history?",
		"Jobs will check the disk alone again, so media you moved or deleted gets downloaded again.", "Cancel", "Clear") == 1 {
		go func() {
			_, err := backend.ClearDownloadHistory()
			a.win.Update(func() {
				if err != nil {
					a.toast(err.Error())
				}
			})
			a.loadHistoryCount()
		}()
	}
}

func tokenHint(saved bool, token string) string {
	switch {
	case !saved:
		return "Not set. Copy the auth_token cookie of x.com from your browser; it stays in the macOS Keychain."
	case len(strings.TrimSpace(token)) != 40:
		return "Saved. (auth_token cookies are usually 40 characters.)"
	}
	return "Saved in the macOS Keychain. Save an empty field to remove it."
}

// extensionDir finds the Chrome extension: in the app's Resources, or
// next to the sources when running from a checkout.
func extensionDir() string {
	if dir, err := mygo.App.Path(mygo.PathResources); err == nil {
		if _, err := os.Stat(filepath.Join(dir, "chrome", "manifest.json")); err == nil {
			return filepath.Join(dir, "chrome")
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, _ = filepath.EvalSymlinks(exe)
	candidates := []string{
		filepath.Join(filepath.Dir(exe), "extension", "chrome"),
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "extension", "chrome"))
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
			return filepath.Clean(dir)
		}
	}
	return ""
}

// updatesOn is set once the updater plugin is in use (not in tests).
var updatesOn bool

// updateSettings is the card of updates: the version, a check now, and
// whether XDL checks, and installs, by itself.
func (a *App) updateSettings(c *ui.Context) {
	if !updatesOn {
		return
	}
	t := c.Theme()
	card(c, "Updates", func() {
		ui.Form(c, func() {
			ui.Field(c, "Version", func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, version.Version).FontSize(13).Bold()
					if v := updater.AvailableVersion(); v != "" {
						ui.Badge(c, v+" is available").Background(t.Accent.Alpha(0.15)).TextColor(t.Accent)
					}
					ui.Spacer(c)
					if ui.Button(c, "Check for Updates…").Disabled(!mygo.Updater.Enabled()).Clicked() {
						updater.CheckForUpdates()
					}
				})
			}).Description(a.updateHint())
			auto := updater.AutomaticChecks()
			ui.Field(c, "Check automatically", func() {
				if ui.Switch(c, &auto).Disabled(!mygo.Updater.Enabled()).Changed() {
					updater.SetAutomaticChecks(auto)
				}
			}).Description("Once a day, and when XDL starts.")
			install := updater.AutomaticDownloads()
			ui.Field(c, "Install updates automatically", func() {
				if ui.Switch(c, &install).Disabled(!mygo.Updater.Enabled() || !auto).Changed() {
					updater.SetAutomaticDownloads(install)
				}
			}).Description("A new version downloads in the background and runs the next time XDL starts.")
		})
	})
}

func (a *App) updateHint() string {
	if !mygo.Updater.Enabled() {
		return "This copy of XDL cannot update itself (a development build, or one installed where it cannot write)."
	}
	if last := updater.LastCheck(); !last.IsZero() {
		return "Last checked " + last.Local().Format("2006-01-02 15:04") + ". Updates are signed; XDL installs only what its release key signed."
	}
	return "Updates are signed; XDL installs only what its release key signed."
}
