package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

const (
	graphSettings = `
direction: right
classes: {
  node: {
    shape: circle
    style {
      font-size: 10
      stroke-width: 1
	}
  }

  edge: {
    style: {
      font-size: 20 
      font-color: black
      italic: true
      stroke-width: 1
    }
  }
}
`
)

type D2 struct {
	script string
}

func NewD2(p *model.PacketPath) (*D2, error) {
	script, err := buildGraphScriptV2(p)
	if err != nil {
		return nil, err
	}

	d := &D2{
		script: script,
	}
	return d, nil
}

func buildGraphScriptV2(p *model.PacketPath) (string, error) {
	builder := strings.Builder{}

	builder.WriteString(graphSettings)
	for _, n := range p.Nodes() {
		builder.WriteString(fmt.Sprintf("%q: %s { class: [node; %s] }\n", n.GetID(), trimID(n.GetID()), n.GetID()))
	}

	for _, l := range p.Links() {
		builder.WriteString(fmt.Sprintf("%q->%q: %s { class: [edge; %s] }\n", l.Source.GetID(), l.Destination.GetID(), l.Type, l.GetID()))
	}

	return builder.String(), nil
}

func (d *D2) ToD2() ([]byte, error) {
	return []byte(d.script), nil
}

func (d *D2) ToSvg() ([]byte, error) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, err
	}
	diag, _, err := d2lib.Compile(context.Background(), d.script, &d2lib.CompileOptions{
		Layout: d2dagrelayout.DefaultLayout,
		Ruler:  ruler,
	})
	if err != nil {
		return nil, err
	}

	out, err := d2svg.Render(diag, &d2svg.RenderOpts{
		Pad: d2svg.DEFAULT_PADDING,
	})

	return out, err
}

func trimID(id string) string {
	ids := strings.Split(id, "/")
	return ids[len(ids)-1]
}
