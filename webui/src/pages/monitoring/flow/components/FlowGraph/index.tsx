import { useRef, useEffect } from "react"
import { Graph } from "@antv/g6"
import { Layout } from "@antv/g6"

interface GraphData {
  combos: string[],
  nodes: any[],
  edges: any[]
  clusterCount: number,
}

interface FlowGraphProps {
  data: any
}

const removeDuplicateEdge = (edges: any[]): any[] => {
  const result: any[] = [];
  const map: any = {};
  edges.forEach((edge: any) => {
    const key = `${edge.source}-${edge.target}`
    const reverseKey = `${edge.target}-${edge.source}`
    if (!map[key] && !map[reverseKey]) {
      result.push(edge)
      map[key] = true
    }
  });
  return result;
}

const getComboName = (n: any): string => {
  switch (n.type) {
    case 'pod':
      return n.namespace
    case 'node':
      return '<Node>'
    case 'external':
      return '<External>'
  }
}

const toGraphData = (data: any): GraphData => {
  let nodes = data.nodes
    .map((item: any) => {
      let label = item.id
      switch (item.type) {
        case 'pod':
          label = `${item.namespace}/${item.name}`
          break
        case 'node':
          label = item.node_name
          break
      }
      return {
        id: item.id,
        label: label,
        comboId: getComboName(item),
        ...item,
      }
    });

  const combos = nodes
    .map(v => v.comboId)
    .filter((i, j, a) => a.indexOf(i) === j)
    .map(v => {
      return {
        id: v,
        label: v,
        padding: 5,
      }
    });

  const edges = removeDuplicateEdge(data.edges.map((item: any) => {
    return {
      id: item.id,
      source: item.src,
      target: item.dst,
    }
  }));

  const clusterCount = nodes.map(i => i.cluster).filter((v, i, a) => a.indexOf(v) === i).length

  return {
    combos,
    nodes,
    edges,
    clusterCount,
  };
}

const FlowGraph: React.FC<FlowGraphProps> = (props: FlowGraphProps): JSX.Element => {
  const ref = useRef(null);
  const { data } = props
  const graphData = data ? toGraphData(data) : null
  useEffect(() => {
    if (!data || !graphData) return;
    const graph = new Graph({
      container: ref.current!,
      width: ref.current!.clientWidth,
      height: ref.current!.clientHeight,
      fitView: true,
      // fitViewPadding: 100,
      groupByTypes: false,
      layout: {
        type: 'comboCombined',
        animate: true,
        outerLayout: new Layout['forceAtlas2']({
          preventOverlap: true,
          kr: 100,
        }),
        innerLayout: new Layout['force2']({
          preventOverlap: true,
          gravity: Math.min(graphData.nodes.length * 150, 2000),
        })
      },
      defaultNode: {
        labelCfg: {
          position: 'bottom',
          style: {
            fontSize: 6,
            fill: 'none',
          },
        }
      },
      defaultEdge: {
        style: {
          radius: 20,
          stroke: '#bec0c2',
          lineWidth: 1.2,
          lineDash: [0, 3, 6],
        },
      },
      defaultCombo: {
      },
      modes: {
        default: [
          {
            type: 'drag-canvas',
            enableOptimize: true,
          },
          {
            type: 'zoom-canvas',
            enableOptimize: true,
            optimizeZoom: 0.01,
          },
          'drag-node',
          'drag-combo',
          // 'shortcuts-call',
        ]
      },
    });

    graph.data(graphData);
    graph.render();

    const graphResize = (entries: ResizeObserverEntry[]) => {
      if (graph) {
        if (!entries || entries.length === 0) return;
        const e = entries[0]
        graph.changeSize(e.target.clientWidth, e.target.clientHeight);
        graph.fitView();
      }
    };

    const ob = new ResizeObserver(graphResize)
    ob.observe(ref.current!);
    return () => { graph?.destroy(); ob.disconnect() };
  }, [props])

  return (
    <div ref={ref} style={{ height: '80vh' }} />
  )
}

export default FlowGraph;
