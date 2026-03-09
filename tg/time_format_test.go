package tg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/tg"
)

func TestTimeHTML(t *testing.T) {
	got := tg.TimeHTML(1647531900, "wDT", "March 17, 2022")
	assert.Equal(t, `<tg-time unix="1647531900" format="wDT">March 17, 2022</tg-time>`, got)
}

func TestTimeHTML_NoFormat(t *testing.T) {
	got := tg.TimeHTML(1647531900, "", "fallback")
	assert.Equal(t, `<tg-time unix="1647531900">fallback</tg-time>`, got)
}

func TestTimeMarkdownV2(t *testing.T) {
	got := tg.TimeMarkdownV2(1647531900, "Dt", "March 17")
	assert.Equal(t, `![March 17](tg://time?unix=1647531900&format=Dt)`, got)
}

func TestTimeMarkdownV2_NoFormat(t *testing.T) {
	got := tg.TimeMarkdownV2(1647531900, "", "fallback")
	assert.Equal(t, `![fallback](tg://time?unix=1647531900)`, got)
}

func TestTimeHTML_ZeroUnix(t *testing.T) {
	got := tg.TimeHTML(0, "d", "epoch")
	assert.Equal(t, `<tg-time unix="0" format="d">epoch</tg-time>`, got)
}

func TestTimeHTML_EmptyFallback(t *testing.T) {
	got := tg.TimeHTML(1647531900, "wDT", "")
	assert.Equal(t, `<tg-time unix="1647531900" format="wDT"></tg-time>`, got)
}

func TestTimeMarkdownV2_ZeroUnix(t *testing.T) {
	got := tg.TimeMarkdownV2(0, "t", "epoch")
	assert.Equal(t, `![epoch](tg://time?unix=0&format=t)`, got)
}
