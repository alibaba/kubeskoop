package ui

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog/v2"
)

//go:embed html
var embeddedWebFiles embed.FS

type uiArgs struct {
	DiagnoseInfo string
	GraphSvg     template.HTML
	Nodes        template.JSStr
	Edges        template.JSStr
}

type suspicion struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type nodeInfo struct {
	ID             string      `json:"id"`
	Type           string      `json:"type"`
	SuspicionLevel string      `json:"suspicion_level"`
	Suspicions     []suspicion `json:"suspicions"`
}

type packet struct {
	Src      string `json:"src"`
	Dst      string `json:"dst"`
	Dport    uint16 `json:"dport"`
	Protocol string `json:"protocol"`
}

type edgeInfo struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Action string `json:"action"`
	Oif    string `json:"oif"`
	Iif    string `json:"iif"`
	Packet packet `json:"packet"`
}

type WebUI struct {
	ctx      *context.Context
	g        *Graphviz
	p        *model.PacketPath
	template *template.Template
	address  string
}

func NewWebUI(ctx *context.Context, p *model.PacketPath, address string) (*WebUI, error) {
	g, err := NewGraphviz(p)
	if err != nil {
		return nil, err
	}

	ui := &WebUI{
		ctx:      ctx,
		p:        p,
		g:        g,
		template: template.New("kubeskoop"),
		address:  address,
	}
	err = ui.loadTemplates()
	if err != nil {
		return nil, err
	}

	return ui, nil
}

func (u *WebUI) loadTemplates() error {
	_, err := u.template.ParseFS(embeddedWebFiles, "html/index.html", "html/default.svg")
	return err
}

func (u *WebUI) Serve() error {
	rtr := mux.NewRouter()
	rtr.HandleFunc("/", u.handleUI)
	rtr.HandleFunc("/svg/{filename}", u.handleSVG)

	http.Handle("/", rtr)
	klog.V(0).Infof("HTTP server listening on http://%s", u.address)
	return http.ListenAndServe(u.address, nil)
}

func (u *WebUI) handleUI(w http.ResponseWriter, req *http.Request) {
	args := uiArgs{
		DiagnoseInfo: fmt.Sprintf("%s -> %s", u.ctx.TaskConfig().SourceEndpoint, u.ctx.TaskConfig().DstEndpoint),
	}

	svg, err := u.g.ToSvg()
	if err != nil {
		http.Error(w, fmt.Sprintf("error generate svg: %s", err), http.StatusInternalServerError)
		return
	}
	svgString := string(svg)
	args.GraphSvg = template.HTML(svgString)

	var nodes []nodeInfo
	for _, node := range u.p.Nodes() {
		n := nodeInfo{
			ID:             node.GetID(),
			Type:           string(node.GetType()),
			SuspicionLevel: node.MaxSuspicionLevel().String(),
			Suspicions:     nil,
		}

		for _, sus := range node.GetSuspicions() {
			n.Suspicions = append(n.Suspicions, suspicion{
				Level:   sus.Level.String(),
				Message: strings.Replace(sus.Message, "\n", "\\n", -1),
			})
		}

		nodes = append(nodes, n)
	}
	jsonBytes, err := json.Marshal(nodes)
	if err != nil {
		http.Error(w, fmt.Sprintf("error marshal node info: %s", err), http.StatusInternalServerError)
		return
	}
	args.Nodes = template.JSStr(jsonBytes)

	var edges []edgeInfo
	for _, link := range u.p.Links() {
		srcAttrs := map[string]string{}
		if link.SourceAttribute != nil {
			srcAttrs = link.SourceAttribute.GetAttrs()
		}
		dstAttrs := map[string]string{}
		if link.DestinationAttribute != nil {
			dstAttrs = link.DestinationAttribute.GetAttrs()
		}

		e := edgeInfo{
			ID:   link.GetID(),
			Type: string(link.Type),
			Packet: packet{
				Src:      link.Packet.Src.String(),
				Dst:      link.Packet.Dst.String(),
				Dport:    link.Packet.Dport,
				Protocol: string(link.Packet.Protocol),
			},
			Oif: "-",
			Iif: "-",
		}

		action := link.Destination.ActionOf(link)
		if action != nil {
			e.Action = string(action.Type)
		}

		if oif, ok := srcAttrs["if"]; ok {
			e.Oif = oif
		}

		if iif, ok := dstAttrs["if"]; ok {
			e.Iif = iif
		}

		edges = append(edges, e)
	}
	jsonBytes, err = json.Marshal(edges)
	if err != nil {
		http.Error(w, fmt.Sprintf("error marshal edge info: %s", err), http.StatusInternalServerError)
		return
	}
	args.Edges = template.JSStr(jsonBytes)

	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	err = u.template.ExecuteTemplate(w, "index.html", &args)
	if err != nil {
		http.Error(w, fmt.Sprintf("error render template: %s", err), http.StatusInternalServerError)
		return
	}
}

func (u *WebUI) handleSVG(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	f, err := embeddedWebFiles.ReadFile(fmt.Sprintf("html/svg/%s", filename))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		http.Error(w, fmt.Sprintf("error get file: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "image/svg+xml")

	if errors.Is(err, fs.ErrNotExist) {
		name := strings.TrimRight(filename, ".svg")
		err = u.template.ExecuteTemplate(w, "default.svg", map[string]string{
			"label": name,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("error render template: %s", err), http.StatusInternalServerError)
		}
		return
	}

	_, _ = w.Write(f)
}
