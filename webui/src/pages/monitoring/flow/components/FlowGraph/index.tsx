import { useRef, useEffect, useMemo, useCallback, useState } from "react"
import { Graph } from "@antv/g6"
import { Layout } from "@antv/g6"
import ForceGraph2D from "react-force-graph-2d";
import { forceManyBody, forceLink } from 'd3-force';
import styles from './index.module.css'
import { clamp } from "@/utils";
import exp from "constants";

interface Node {
  id: string
  name: string
  type: string
  group: string
  links: Link[]
}

interface Link {
  source: string
  target: string
  edges: any[]
}

interface GraphData {
  nodes: Node[]
  links: Link[]
}


interface FlowGraphProps {
  data: any
}

interface GroupInfo {
  name: string
  position: [number, number]
}

const parsePodName = (n: any): {
  group: string
  groupType: string
} => {
  // parse pod, deployment or daemonset by pod name
  const parts = n.name.split('-');
  if (/^.+-([a-z0-9]{5,10})-[a-z0-9]{5,10}$/.test(n.name)) {
    return {
      group: `${n.namespace}/${parts.slice(0, parts.length - 2).join('-')}`,
      groupType: 'deployment',
    }
  }
  if (/^.+-[a-z0-9]{5,10}$/.test(n.name)) {
    return {
      group: `${n.namespace}/${parts.slice(0, parts.length - 1).join('-')}`,
      groupType: 'daemonset',
    }
  }

  return {
    group: `${n.namespace}/${n.name}`,
    groupType: 'pod',
  }
}

const getGroupData = (n: any): any => {
  switch (n.type) {
    case 'pod':
      return {
        ...parsePodName(n)
      }
    case 'node':
      return {
        group: 'Node',
        groupType: 'node',
      }
    case 'external':
      return {
        group: 'External',
        groupType: 'endpoint',
      }
    default:
      return {
        group: 'Unknown',
        groupType: 'endpoint',
      }
  }
}

const addVirtualNodeAndLinks = (data: GraphData, groups: string[]) => {
  data.nodes.filter(n => groups.includes(n.group)).forEach((n) => {
    data.links.push({
      source: `!${n.group}`,
      target: n.id,
      edges: [],
      type: 'virtual'
    })
  });

  groups.forEach((g) => {
    data.nodes.push({
      id: `!${g}`,
      name: '',
      type: 'virtual',
      group: '_',
      links: [],
      val: 0.1,
    })
  });

  return data;
}

const toName = (n: any): string => {
  switch (n.type) {
    case 'node':
      return n.node_name
    case 'pod':
      return n.name
    default:
      return n.id
  }
}

const toGroupedGraphData = (data: any, expandedGroups: GroupInfo[]): GraphData => {
  const withGroup = data.nodes.map((n) => {
    return {
      id: n.id,
      name: toName(n),
      links: [],
      type: n.type,
      ...getGroupData(n)
    }
  });

  const originalNodeMap = {};
  withGroup.forEach((n) => {
    originalNodeMap[n.id] = n;
  });

  const groups = new Set<string>(withGroup.map(n => n.group).filter(g => !expandedGroups.find(n => n.name === g)));

  const nodeMap = {}
  const nodes = Array.from(groups).map((g) => {
    return {
      id: g,
      name: g,
      group: g,
      type: 'group',
      groupType: withGroup.find(n => n.group === g).groupType,
      links: [],
      // nodes: withGroup.filter(n => n.group === g),
    }
  });

  expandedGroups.forEach((g) => {
    nodes.push(...withGroup.filter(n => n.group === g.name))
  });


  nodes.forEach((n) => {
    nodeMap[n.id] = n
  });

  const links: any[] = [];
  const linkMap = {};
  data.edges.forEach((i) => {
    let srcNode = originalNodeMap[i.src];
    if (groups.has(srcNode.group)) {
      srcNode = nodeMap[srcNode.group];
    } else {
      const grp = expandedGroups.find(n => n.name === srcNode.group);
      srcNode.x = grp?.position[0];
      srcNode.y = grp?.position[1];
    }
    let dstNode = originalNodeMap[i.dst];
    if (groups.has(dstNode.group)) {
      dstNode = nodeMap[dstNode.group];
    } else {
      const grp = expandedGroups.find(n => n.name === dstNode.group);
      dstNode.x = grp?.position[0];
      dstNode.y = grp?.position[1];
    }

    // construct links from group to group
    const key = `${srcNode.id}:${dstNode.id}`;
    const reverseKey = `${dstNode.id}:${srcNode.id}`;
    if (linkMap[key]) {
      linkMap[key].edges.push(i);
      return;
    }

    let l = {
      id: key,
      name: key,
      source: srcNode.id,
      target: dstNode.id,
      edges: [],
    }

    linkMap[key] = l
    linkMap[reverseKey] = l

    nodeMap[l.source].links.push(l)
    nodeMap[l.target].links.push(l)
    links.push(l)
  })

  const ret = addVirtualNodeAndLinks({
    nodes,
    links
  }, expandedGroups.map(g => g.name));

  return ret
}

const drawNode = (node, ctx: CanvasRenderingContext2D, globalScale, highlight, hide) => {
  if (highlight) {
    ctx.beginPath();
    ctx.fillStyle = "red";
    ctx.arc(node.x, node.y, 2 * 1.2, 0, Math.PI * 2)
    ctx.fill();
  }

  // draw node
  ctx.beginPath();
  ctx.fillStyle = hide ? node.color + '99' : node.color;
  ctx.arc(node.x, node.y, 2, 0, Math.PI * 2);
  ctx.fill();

  if (hide) {
    return;
  }


  const label = node.name;
  const fontSize = 12 / globalScale;
  ctx.font = `${fontSize}px Sans-Serif`;
  const textWidth = ctx.measureText(label).width;
  const bckgDimensions = [textWidth, fontSize].map(n => n + fontSize * 0.2); // some padding

  ctx.fillStyle = 'rgba(255, 255, 255, 0.6)';
  ctx.fillRect(node.x - bckgDimensions[0] / 2, node.y + 3 - bckgDimensions[1] / 2, ...bckgDimensions);

  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillStyle = "#000000";
  ctx.fillText(label, node.x, node.y + 3);

  node.__bckgDimensions = bckgDimensions; // to re-use in nodePointerAreaPaint
}

const drawLink = (link, ctx, globalScale) => {
  const SPEED = link.edges.length * 0.2;
  const PARTICLE_SIZE = 2 / globalScale;
  const start = link.source;
  const end = link.target;

  const diff = { x: end.x - start.x, y: end.y - start.y };
  const length = Math.sqrt(diff.x * diff.x + diff.y * diff.y);
  diff.x /= length;
  diff.y /= length;

  const mod = clamp(1000 * 1 / SPEED, 500, 5000);

  // add random offsets to particles
  if (!link.offset) {
    link.offset = Math.random() * mod;
    link.offset2 = Math.random() * mod;
  }

  const t = ((Date.now() + link.offset) % mod) / mod;
  const t2 = ((Date.now() + link.offset2) % mod) / mod;
  const pos = {
    x: start.x + diff.x * t * length,
    y: start.y + diff.y * t * length
  };
  const pos2 = {
    x: start.x + diff.x * (1 - t2) * length,
    y: start.y + diff.y * (1 - t2) * length
  };


  ctx.beginPath();
  ctx.arc(pos.x, pos.y, PARTICLE_SIZE, 0, Math.PI * 2);
  ctx.fillStyle = start.color;
  ctx.fill();

  ctx.beginPath();
  ctx.arc(pos2.x, pos2.y, PARTICLE_SIZE, 0, Math.PI * 2);
  ctx.fillStyle = end.color;
  ctx.fill();
}

const nodeLabel = (n) => {
  const label = [
    `Name: ${n.name}`,
    `Type: ${n.type === 'group' ? n.groupType : n.type}`
  ]
  return label.join("</br>")
}

const linkLabel = (l) => {
  const label = [
    `Connection(s): ${l.edges.length}`,
    `Send Packets: ${l.edges.reduce((a, b) => a + b.packets, 0)}`,
    `Send Byte(s): ${l.edges.reduce((a, b) => a + b.bytes, 0)}`,
    `Dropped Packet(s): ${l.edges.reduce((a, b) => a + b.dropped, 0)}`,
    `Retransmitted Packet(s): ${l.edges.reduce((a, b) => a + b.retrans, 0)}`
  ]
  return label.join("</br>")
}

const FlowGraphD3: React.FC<FlowGraphProps> = (props: FlowGraphProps): JSX.Element => {
  const fgRef = useRef<any>(null);
  const [expanded, setExpanded] = useState<GroupInfo[]>([])
  const data = useMemo(() => toGroupedGraphData(props.data, expanded), [props, expanded]);

  useEffect(() => {
    if (fgRef.current) {
      const fg = fgRef.current;

      fg.d3Force('charge', forceManyBody()
        .strength(node => {
          return node.type === 'virtual' ? 0 : -15;
        }));

      fg.d3Force('link', forceLink().id(d => d.id)
        .distance(link => {
          const sameGroup = link.source.group === link.target.group;
          if (link.type !== 'virtual') return sameGroup ? 10 : 40;
          return 0;
        })
      );

      fg.d3ReheatSimulation();
    }
  }, []);

  const visibility = (d) => {
    return d.type !== 'virtual';
  }

  const [highlightNodes, setHighlightNodes] = useState(new Set())
  const [highlightLinks, setHighlightLinks] = useState(new Set())

  const addHighlight = (n, l) => {
    const newNodes = new Set()
    const newLinks = new Set()

    if (n) {
      newNodes.add(n.id)
      n.links.forEach(l => {
        newNodes.add(l.source.id)
        newNodes.add(l.target.id)
        newLinks.add(l.id)
      })
    }

    if (l) {
      newLinks.add(l.id)
      newNodes.add(l.source.id)
      newNodes.add(l.target.id)
    }

    setHighlightNodes(newNodes)
    setHighlightLinks(newLinks)
  }

  const drawNodeCallback = useCallback((n, ctx, scale) => {
    if (highlightNodes.size != 0) {
      const h = highlightNodes.has(n.id)
      drawNode(n, ctx, scale, h, !h)
    } else {
      drawNode(n, ctx, scale, false, false)
    }
  }, [highlightNodes])

  const drawLinkCallback = useCallback((l, ctx, scale) => {
    if (highlightLinks.size != 0) {
      const h = highlightLinks.has(l.id)
      h && drawLink(l, ctx, scale)
    } else {
      drawLink(l, ctx, scale)
    }
  }, [highlightLinks])

  return <div className={styles.flowGraph}>
    <ForceGraph2D
      width={window.innerWidth * 0.8}
      height={window.innerHeight * 0.75}
      ref={fgRef}
      graphData={data}
      linkWidth={(l) => clamp(l.edges.length * 0.01, highlightLinks.has(l.id) ? 2 : 1, 5)}
      autoPauseRedraw={false}
      linkCanvasObjectMode={() => "after"}
      linkCanvasObject={drawLinkCallback}
      linkColor={(l) => highlightLinks.size != 0 ? highlightLinks.has(l.id) ? "#746aff" : "#EEE" : "#999"}
      linkVisibility={visibility}
      onLinkHover={(l) => addHighlight(null, l)}
      nodeCanvasObject={drawNodeCallback}
      nodeVisibility={visibility}
      nodeRelSize={2}
      onNodeDragEnd={node => {
        node.fx = node.x;
        node.fy = node.y;
      }}
      onNodeHover={(n) => addHighlight(n, null)}
      onNodeClick={(n) => setExpanded(n.type === 'group' ? [{ name: n.group, position: [n.x, n.y] }, ...expanded] : expanded.filter(g => g.name !== n.group))}
      nodeLabel={nodeLabel}
      linkLabel={linkLabel}
      onEngineStop={() => {fgRef.current.zoomToFit(1000)}}
      cooldownTime={3000}
      nodeAutoColorBy={'group'}
    >
    </ForceGraph2D>
    <div>Hover to show node/link detail, and click to expand the node.</div>
  </div >
}

export default FlowGraphD3;
