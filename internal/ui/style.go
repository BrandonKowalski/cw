package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func dim(s string) string {
	return lipgloss.NewStyle().Faint(true).Render(s)
}

func red(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render(s)
}

func yellow(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render(s)
}

func green(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(s)
}

func heading(s string) string {
	return lipgloss.NewStyle().Bold(true).Render(s)
}

func FormatCount(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		return "-" + out
	}
	return out
}

func FormatSize(n int64) string {
	if n < 0 {
		return "-" + FormatSize(-n)
	}
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, s := range []string{"kB", "MB", "GB", "TB"} {
		v /= unit
		if v < unit || s == "TB" {
			return fmt.Sprintf("%.1f %s", v, s)
		}
	}
	return ""
}

func FormatDuration(d time.Duration) string {
	secs := int(d.Round(time.Second) / time.Second)
	switch {
	case secs >= 3600:
		return fmt.Sprintf("%dh%02dm%02ds", secs/3600, secs%3600/60, secs%60)
	case secs >= 60:
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%ds", secs)
}
