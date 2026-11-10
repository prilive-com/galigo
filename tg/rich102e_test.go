package tg_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 rich message media: the media list and the five media blocks (B-25 10.2e, TEST-PLAN §2).

// fake102e is a test-local tg.InputRichMedia (tg has no implementation: sender.InputMedia is the real one).
type fake102e struct {
	Type  string `json:"type"`
	Media string `json:"media"`
}

func (f fake102e) InputRichMediaType() string { return f.Type }

func cap102e(text, credit string) *tg.RichBlockCaption {
	c := &tg.RichBlockCaption{Text: tg.RichText{Plain: text}}
	if credit != "" {
		c.Credit = &tg.RichText{Plain: credit}
	}
	return c
}

// rich102eKinds: the five media blocks, built from a media value and a caption.
var rich102eKinds = []struct {
	goType, constant, key string
	make                  func(m tg.InputRichMedia, c *tg.RichBlockCaption) tg.InputRichBlock
}{
	{"InputRichBlockAnimation", "animation", "animation", func(m tg.InputRichMedia, c *tg.RichBlockCaption) tg.InputRichBlock {
		return tg.InputRichBlockAnimation{Animation: m, Caption: c}
	}},
	{"InputRichBlockAudio", "audio", "audio", func(m tg.InputRichMedia, c *tg.RichBlockCaption) tg.InputRichBlock {
		return tg.InputRichBlockAudio{Audio: m, Caption: c}
	}},
	{"InputRichBlockPhoto", "photo", "photo", func(m tg.InputRichMedia, c *tg.RichBlockCaption) tg.InputRichBlock {
		return tg.InputRichBlockPhoto{Photo: m, Caption: c}
	}},
	{"InputRichBlockVideo", "video", "video", func(m tg.InputRichMedia, c *tg.RichBlockCaption) tg.InputRichBlock {
		return tg.InputRichBlockVideo{Video: m, Caption: c}
	}},
	{"InputRichBlockVoiceNote", "voice_note", "voice_note", func(m tg.InputRichMedia, c *tg.RichBlockCaption) tg.InputRichBlock {
		return tg.InputRichBlockVoiceNote{VoiceNote: m, Caption: c}
	}},
}

// Compile-time: each media block is an InputRichBlock, and the fake is an InputRichMedia.
var (
	_ tg.InputRichBlock = tg.InputRichBlockAnimation{}
	_ tg.InputRichBlock = tg.InputRichBlockAudio{}
	_ tg.InputRichBlock = tg.InputRichBlockPhoto{}
	_ tg.InputRichBlock = tg.InputRichBlockVideo{}
	_ tg.InputRichBlock = tg.InputRichBlockVoiceNote{}
	_ tg.InputRichMedia = fake102e{}
)

// T-kind-e: five kinds, distinct names and constants; "type" first and once, for the value and its pointer.
func TestAPI102E_Kinds(t *testing.T) {
	require.Len(t, rich102eKinds, 5)
	names, consts := map[string]bool{}, map[string]bool{}
	for _, k := range rich102eKinds {
		t.Run(k.constant, func(t *testing.T) {
			assert.False(t, names[k.goType] || consts[k.constant], "distinct")
			names[k.goType], consts[k.constant] = true, true
			b := k.make(fake102e{Type: "x", Media: "M"}, nil)
			assert.Equal(t, k.goType, reflect.TypeOf(b).Name())
			ptr := reflect.New(reflect.TypeOf(b))
			ptr.Elem().Set(reflect.ValueOf(b))
			for _, v := range []any{b, ptr.Interface()} {
				out, err := json.Marshal(v)
				require.NoError(t, err)
				assert.True(t, strings.HasPrefix(string(out), `{"type":"`+k.constant+`"`), string(out))
				assert.Equal(t, 1, strings.Count(string(out), `"type":"`+k.constant+`"`), string(out))
				var probe struct{ Type string }
				require.NoError(t, json.Unmarshal(out, &probe))
				assert.Equal(t, k.constant, probe.Type)
			}
		})
	}
}

// T-json-e: the exact bytes of each block: full, minimal, as given, empty caption, nil media, typed nil media, a
// pointer block, and the block inside each container.
func TestAPI102E_BlockJSON(t *testing.T) {
	var typedNil *fake102e
	for _, k := range rich102eKinds {
		m := fake102e{Type: k.constant + "-t", Media: k.constant + "-m"}
		mj := `{"type":"` + k.constant + `-t","media":"` + k.constant + `-m"}`
		pre := `{"type":"` + k.constant + `","` + k.key + `":`
		rows := []struct {
			name string
			in   any
			want string
		}{
			{"full", k.make(m, cap102e("c-"+k.constant, "r-"+k.constant)),
				pre + mj + `,"caption":{"text":"c-` + k.constant + `","credit":"r-` + k.constant + `"}}`},
			{"min", k.make(m, nil), pre + mj + `}`},
			{"as-given", k.make(fake102e{Type: " t\t", Media: "\tm "}, cap102e(" c\t", "\tr ")),
				pre + `{"type":" t\t","media":"\tm "},"caption":{"text":" c\t","credit":"\tr "}}`},
			{"empty-caption", k.make(m, &tg.RichBlockCaption{}), pre + mj + `,"caption":{"text":""}}`},
			{"nil-media", k.make(nil, nil), pre + `null}`},
			{"typed-nil-media", k.make(typedNil, nil), pre + `null}`},
			{"pointer-media", k.make(&m, nil), pre + mj + `}`},
		}
		for _, r := range rows {
			t.Run(k.constant+"/"+r.name, func(t *testing.T) {
				out, err := json.Marshal(r.in)
				require.NoError(t, err)
				assert.Equal(t, r.want, string(out))
				p := reflect.New(reflect.TypeOf(r.in))
				p.Elem().Set(reflect.ValueOf(r.in))
				outp, err := json.Marshal(p.Interface())
				require.NoError(t, err)
				assert.Equal(t, r.want, string(outp), "pointer block")
			})
		}
		// Inside each of the five containers the media block encodes unchanged (media blocks are leaves).
		b := k.make(m, nil)
		bj := pre + mj + `}`
		containers := []struct {
			name string
			in   tg.InputRichBlock
			want string
		}{
			{"collage", tg.InputRichBlockCollage{Blocks: []tg.InputRichBlock{b, b}}, `{"type":"collage","blocks":[` + bj + `,` + bj + `]}`},
			{"slideshow", tg.InputRichBlockSlideshow{Blocks: []tg.InputRichBlock{b}}, `{"type":"slideshow","blocks":[` + bj + `]}`},
			{"blockquote", tg.InputRichBlockBlockQuotation{Blocks: []tg.InputRichBlock{b}}, `{"type":"blockquote","blocks":[` + bj + `]}`},
			{"details", tg.InputRichBlockDetails{Summary: tg.RichText{Plain: "s"}, Blocks: []tg.InputRichBlock{b}},
				`{"type":"details","summary":"s","blocks":[` + bj + `]}`},
			{"list", tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: []tg.InputRichBlock{b}}}},
				`{"type":"list","items":[{"blocks":[` + bj + `]}]}`},
		}
		for _, c := range containers {
			t.Run(k.constant+"/in-"+c.name, func(t *testing.T) {
				out, err := json.Marshal(c.in)
				require.NoError(t, err)
				assert.Equal(t, c.want, string(out))
			})
		}
	}
}

// T-mm: the media list item and InputRichMessage.Media.
func TestAPI102E_MessageMedia(t *testing.T) {
	var typedNil *fake102e
	m := fake102e{Type: "photo", Media: "P"}
	mj := `{"type":"photo","media":"P"}`
	rows := []struct {
		name string
		in   any
		want string
	}{
		{"item", tg.InputRichMessageMedia{ID: "p1", Media: m}, `{"id":"p1","media":` + mj + `}`},
		{"item/as-given", tg.InputRichMessageMedia{ID: " p\t", Media: m}, `{"id":" p\t","media":` + mj + `}`},
		{"item/zero", tg.InputRichMessageMedia{}, `{"id":"","media":null}`},
		{"item/typed-nil", tg.InputRichMessageMedia{ID: "n", Media: typedNil}, `{"id":"n","media":null}`},
		{"item/pointer", tg.InputRichMessageMedia{ID: "q", Media: &m}, `{"id":"q","media":` + mj + `}`},
		{"message", tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{{ID: "a", Media: m}, {ID: "b", Media: m}}},
			`{"markdown":"m","media":[{"id":"a","media":` + mj + `},{"id":"b","media":` + mj + `}]}`},
		{"message/all", tg.InputRichMessage{HTML: "h", Markdown: "m", IsRTL: true, SkipEntityDetection: true,
			Blocks: []tg.InputRichBlock{tg.InputRichBlockDivider{}}, Media: []tg.InputRichMessageMedia{{ID: "a", Media: m}}},
			`{"html":"h","markdown":"m","is_rtl":true,"skip_entity_detection":true,"blocks":[{"type":"divider"}],"media":[{"id":"a","media":` + mj + `}]}`},
		{"message/empty", tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{}}, `{"markdown":"m"}`},
		{"message/nil", tg.InputRichMessage{Markdown: "m"}, `{"markdown":"m"}`},
		{"inline", tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{{ID: "a", Media: m}}}},
			`{"rich_message":{"markdown":"m","media":[{"id":"a","media":` + mj + `}]}}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// rich102eShape: every field of the five blocks and InputRichMessageMedia {Go name, Go type, json tag}.
var rich102eShape = map[string][]rich101dField{
	"InputRichBlockAnimation": {{"Animation", "tg.InputRichMedia", "animation"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockAudio":     {{"Audio", "tg.InputRichMedia", "audio"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockPhoto":     {{"Photo", "tg.InputRichMedia", "photo"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockVideo":     {{"Video", "tg.InputRichMedia", "video"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichBlockVoiceNote": {{"VoiceNote", "tg.InputRichMedia", "voice_note"}, {"Caption", "*tg.RichBlockCaption", "caption,omitempty"}},
	"InputRichMessageMedia":   {{"ID", "string", "id"}, {"Media", "tg.InputRichMedia", "media"}},
}

// T-shape-e
func TestAPI102E_Shapes(t *testing.T) {
	values := map[string]any{"InputRichMessageMedia": tg.InputRichMessageMedia{}}
	for _, k := range rich102eKinds {
		values[k.goType] = k.make(nil, nil)
	}
	require.Len(t, rich102eShape, 6)
	require.Len(t, values, 6)
	for name, want := range rich102eShape {
		t.Run(name, func(t *testing.T) {
			v, ok := values[name]
			require.True(t, ok)
			rtyp := reflect.TypeOf(v)
			assert.Equal(t, want, rich101dFields(t, v))
			require.Equal(t, len(want), rtyp.NumField())
			for i := range rtyp.NumField() {
				assert.False(t, rtyp.Field(i).Anonymous, rtyp.Field(i).Name)
			}
			methods := 1
			if name == "InputRichMessageMedia" {
				methods = 0
			}
			require.Equal(t, methods, rtyp.NumMethod())
			require.Equal(t, methods, reflect.PointerTo(rtyp).NumMethod())
			if methods == 1 {
				assert.Equal(t, "MarshalJSON", rtyp.Method(0).Name)
			}
		})
	}
}

// T-iface: tg.InputRichMedia is a marker interface with one exported method; it is not a json.Marshaler (a typed nil
// would panic in encoding/json, plan D1); no tg type implements it (sender.InputMedia is the implementation).
func TestAPI102E_InputRichMediaInterface(t *testing.T) {
	it := reflect.TypeOf((*tg.InputRichMedia)(nil)).Elem()
	require.Equal(t, 1, it.NumMethod())
	assert.Equal(t, "InputRichMediaType", it.Method(0).Name)
	assert.Empty(t, it.Method(0).PkgPath, "the method is exported")
	assert.False(t, it.Implements(reflect.TypeOf((*json.Marshaler)(nil)).Elem()), "not a json.Marshaler")
	assert.Equal(t, "photo", tg.InputRichMedia(fake102e{Type: "photo"}).InputRichMediaType())

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	var impl []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Name.Name == "InputRichMediaType" {
				impl = append(impl, f)
			}
		}
	}
	assert.Empty(t, impl, "a tg type declares InputRichMediaType")
}

// T-walk (E-0132): the per-field rules are listed by a reflection walk over the new types, not by hand. Every string
// reachable through struct and pointer fields (a RichText is a leaf: its Plain is set) is set to a distinct value with
// edge whitespace and must appear once, as given; every interface field gets a fake with such values. Every
// "omitempty" field left at its zero value must be absent, every other field present.
func TestAPI102E_FieldWalk(t *testing.T) {
	values := map[string]any{"InputRichMessageMedia": tg.InputRichMessageMedia{}}
	for _, k := range rich102eKinds {
		values[k.goType] = k.make(nil, nil)
	}
	richText := reflect.TypeOf(tg.RichText{})
	mediaType := reflect.TypeOf((*tg.InputRichMedia)(nil)).Elem()
	var leaves []string
	for name, v := range values {
		t.Run(name, func(t *testing.T) {
			n := 0
			next := func() string { n++; return " v" + strconv.Itoa(n) + "\t" }
			var want []string
			var fill func(path string, v reflect.Value)
			fill = func(path string, v reflect.Value) {
				switch {
				case v.Type() == richText:
					s := next()
					v.Set(reflect.ValueOf(tg.RichText{Plain: s}))
					want = append(want, s)
					leaves = append(leaves, path)
				case v.Kind() == reflect.String:
					s := next()
					v.SetString(s)
					want = append(want, s)
					leaves = append(leaves, path)
				case v.Type() == mediaType:
					f := fake102e{Type: next(), Media: next()}
					v.Set(reflect.ValueOf(f))
					want = append(want, f.Type, f.Media)
					leaves = append(leaves, path+".Type", path+".Media")
				case v.Kind() == reflect.Pointer && v.Type().Elem().Kind() == reflect.Struct:
					v.Set(reflect.New(v.Type().Elem()))
					fill(path, v.Elem())
				case v.Kind() == reflect.Struct:
					for i := range v.NumField() {
						fill(path+"."+v.Type().Field(i).Name, v.Field(i))
					}
				default:
					t.Fatalf("%s: field kind %s has no rule", path, v.Kind())
				}
			}
			full := reflect.New(reflect.TypeOf(v)).Elem()
			fill(name, full)
			out, err := json.Marshal(full.Interface())
			require.NoError(t, err)
			for _, s := range want {
				enc, _ := json.Marshal(s)
				assert.Equal(t, 1, strings.Count(string(out), string(enc)), "%s sent as given: %s", s, out)
			}
			// Zero values: an omitempty field is absent, every other field present.
			zero, err := json.Marshal(v)
			require.NoError(t, err)
			var keys map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(zero, &keys))
			rtyp := reflect.TypeOf(v)
			for i := range rtyp.NumField() {
				name, opts, _ := strings.Cut(rtyp.Field(i).Tag.Get("json"), ",")
				_, present := keys[name]
				assert.Equal(t, opts != "omitempty", present, "%s: key %q present at zero value", rtyp.Field(i).Name, name)
			}
		})
	}
	// The walk itself: 5 blocks × (media type, media, caption text, caption credit) + the item's id, media type, media.
	slices.Sort(leaves)
	require.Len(t, leaves, 23, "%v", leaves)
	assert.Contains(t, leaves, "InputRichMessageMedia.ID")
	assert.Contains(t, leaves, "InputRichBlockPhoto.Caption.Credit")
}
