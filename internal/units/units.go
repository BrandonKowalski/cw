package units

import "fmt"

func Size(n int64) string {
	if n < 0 {
		return "-" + Size(-n)
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
