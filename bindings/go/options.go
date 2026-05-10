package fulgur

import "encoding/json"

// PageSize selects a named ISO size or a custom width/height in mm.
type PageSize struct {
	Name     string  `json:"-"`
	WidthMM  float32 `json:"widthMm,omitempty"`
	HeightMM float32 `json:"heightMm,omitempty"`
}

// MarshalJSON emits either a string ("A4", "Letter", "A3") or an object
// {widthMm, heightMm}, matching the wasm-side PageSizeOption variants.
func (p PageSize) MarshalJSON() ([]byte, error) {
	if p.Name != "" {
		return json.Marshal(p.Name)
	}
	type custom struct {
		W float32 `json:"widthMm"`
		H float32 `json:"heightMm"`
	}
	return json.Marshal(custom{W: p.WidthMM, H: p.HeightMM})
}

// Margin selects a uniform mm/pt margin or per-side mm margins. Set
// exactly one of (UniformMM, UniformPT, Per).
type Margin struct {
	UniformMM *float32     `json:"-"`
	UniformPT *float32     `json:"-"`
	Per       *MarginPerMM `json:"-"`
}

// MarginPerMM is the per-side variant.
type MarginPerMM struct {
	TopMM    float32 `json:"topMm"`
	RightMM  float32 `json:"rightMm"`
	BottomMM float32 `json:"bottomMm"`
	LeftMM   float32 `json:"leftMm"`
}

// MarshalJSON emits one of {mm}, {pt}, or {topMm, rightMm, bottomMm, leftMm}.
func (m Margin) MarshalJSON() ([]byte, error) {
	switch {
	case m.UniformMM != nil:
		return json.Marshal(struct {
			MM float32 `json:"mm"`
		}{*m.UniformMM})
	case m.UniformPT != nil:
		return json.Marshal(struct {
			PT float32 `json:"pt"`
		}{*m.UniformPT})
	case m.Per != nil:
		return json.Marshal(m.Per)
	default:
		return []byte("null"), nil
	}
}

// Options mirrors the Rust EngineOptions JSON shape (camelCase fields,
// deny_unknown_fields on the wasm side). All fields are optional and
// behave as partial sticky overrides on the wasm-side EngineState.
type Options struct {
	PageSize     *PageSize `json:"pageSize,omitempty"`
	Margin       *Margin   `json:"margin,omitempty"`
	Landscape    *bool     `json:"landscape,omitempty"`
	Title        string    `json:"title,omitempty"`
	Authors      []string  `json:"authors,omitempty"`
	Description  string    `json:"description,omitempty"`
	Keywords     []string  `json:"keywords,omitempty"`
	Creator      string    `json:"creator,omitempty"`
	Producer     string    `json:"producer,omitempty"`
	CreationDate string    `json:"creationDate,omitempty"`
	Lang         string    `json:"lang,omitempty"`
	Bookmarks    *bool     `json:"bookmarks,omitempty"`
}
