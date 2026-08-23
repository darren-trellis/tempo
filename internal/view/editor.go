package view

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func resolveEditor() (string, []string, error) {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parts := strings.Fields(value)
		if path, err := exec.LookPath(parts[0]); err == nil {
			return path, parts[1:], nil
		}
		return parts[0], parts[1:], nil
	}
	for _, name := range []string{"nvim", "vim", "nano", "vi"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil, nil
		}
	}
	return "", nil, fmt.Errorf("set $VISUAL or $EDITOR")
}

func editorFileExt(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return ".json"
	}
	return ".txt"
}

func writeEditorFile(label, content string) (string, error) {
	formatted := formatJSONPretty(content)
	f, err := os.CreateTemp("", "tempo-"+label+"-*"+editorFileExt(formatted))
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(formatted); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func openInEditor(app *App, label, content string) {
	if app == nil {
		return
	}
	if strings.TrimSpace(content) == "" {
		app.ToastError("No " + label + " to open")
		return
	}
	bin, args, err := resolveEditor()
	if err != nil {
		app.ToastError("No editor: " + err.Error())
		return
	}
	path, err := writeEditorFile(label, content)
	if err != nil {
		app.ToastError("Failed to write temp file: " + err.Error())
		return
	}
	defer os.Remove(path)

	jig := app.JigApp()
	if jig == nil {
		app.ToastError("Editor unavailable")
		return
	}
	var runErr error
	if !jig.Suspend(func() {
		cmd := exec.Command(bin, append(append([]string{}, args...), path)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		runErr = cmd.Run()
	}) {
		app.ToastError("Failed to suspend terminal for editor")
		return
	}
	if runErr != nil {
		app.ToastError("Editor failed: " + runErr.Error())
	}
}
