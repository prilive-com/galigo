package tg

import "fmt"

// TimeHTML returns an HTML date/time string for Telegram.
// Format tokens: r=relative, w=weekday, d=short date, D=long date, t=short time, T=long time.
// The format parameter must be a valid Telegram format string (e.g. "wDT"), not arbitrary user input.
// The fallbackText is shown to clients that don't support tg-time and is NOT HTML-escaped.
// Example: TimeHTML(1647531900, "wDT", "March 17, 2022 15:45") produces
// <tg-time unix="1647531900" format="wDT">March 17, 2022 15:45</tg-time>
func TimeHTML(unix int64, format, fallbackText string) string {
	if format == "" {
		return fmt.Sprintf(`<tg-time unix="%d">%s</tg-time>`, unix, fallbackText)
	}
	return fmt.Sprintf(`<tg-time unix="%d" format=%q>%s</tg-time>`, unix, format, fallbackText)
}

// TimeMarkdownV2 returns a MarkdownV2 date/time string for Telegram.
// The fallbackText must not contain unescaped MarkdownV2 special characters (e.g. ], [, )).
// The format parameter must be a valid Telegram format string (e.g. "Dt").
// Example: TimeMarkdownV2(1647531900, "wDT", "March 17") produces
// ![March 17](tg://time?unix=1647531900&format=wDT)
func TimeMarkdownV2(unix int64, format, fallbackText string) string {
	if format == "" {
		return fmt.Sprintf(`![%s](tg://time?unix=%d)`, fallbackText, unix)
	}
	return fmt.Sprintf(`![%s](tg://time?unix=%d&format=%s)`, fallbackText, unix, format)
}
