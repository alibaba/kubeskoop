import G6, { IGroup, ModelConfig, NodeConfig } from "@antv/g6";

export default function registerDiagnosisNode() {
  G6.registerNode("diagnosis-node", {
    afterDraw(cfg, group, rst) {
        console.log(rst)
        const size = this.getSize(cfg);
        const r = size[0] / 2;

        const style = (cfg!.labelCfg && cfg!.labelCfg.style) || {};
        style.text = cfg!.label;
        const t = group!.getChildren().find(i => i.cfg.className == 'node-label')
        if (t) {
          t.attrs = {
            ...t.attrs,
            textBaseline: 'top',
          }
        }

        const s = 0.8*r;
        const img = group!.addShape('image', {
          attrs: {
            cursor: 'pointer',
            img: cfg.nodeData.img,
            x: -s/2,
            y: -s-(r-s)/2,
            height: s,
            width: s,
          },
          name: 'node-type-img'
        });

        const si = 0.3*r;
        const indr = group!.addShape("rect", {
          attrs: {
            cursor: 'pointer',
            height: si,
            width: si,
            x: -si/2,
            // y: t?.getBBox().height + 2,
            y: t?.attrs.fontSize * 2 + 5,
            radius: 4,
            text: '1',
            fill: cfg.nodeData.color,
          },
          name: 'indicator-rect'
        });

      const indt = group!.addShape("text", {
          attrs: {
            cursor: 'pointer',
            y: indr.attrs.y + 0.5*si,
            fontSize: 0.7*si,
            fontWeight: 'bold',
            text: cfg.nodeData.count.toString(),
            fill: 'white',
            textBaseline: 'middle',
            textAlign: 'center'
          },
          name: 'indicator-text'
        });

    },
    update: undefined,
  }, 'circle')
};
