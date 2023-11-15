import { Graph, GraphData } from "@antv/g6";
import { useEffect, useRef } from "react";
import registerDiagnosisNode from "./node";

const suspicionColors = {
  0: '#30BD61',
  1: '#FFB369',
  2: '#F76D76',
  3: '#F76D76'
}

const nodeTypeIcon = {
  'pod': '/img/pod.svg',
  'node': '/img/node.svg',
}

interface DiagnosisResultProps {
  data?: DiagnosisResultData;
  onClick?: (type: "node" | "edge", id: string) => void
};

const wrapLine = (text: string, wrapCount: number): string => {
  if (text.length < wrapCount) return text;
  return text.substring(0, wrapCount) + '\n' + wrapLine(text.substring(wrapCount), wrapCount)
}

const maxSuspicionLevel = (sus: Suspicion[]): SuspicionLevel => {
  return Math.max(...sus.map(i => i.level))
}

const toGraphData = (data: DiagnosisResultData): GraphData => {
  let graphData: GraphData = {}

  if (data.nodes) {
    graphData.nodes = data.nodes.map(node => {
      const nodeData = {
        color: node.suspicions && node.suspicions.length > 0 ?
          suspicionColors[maxSuspicionLevel(node.suspicions)] : suspicionColors[0],
        count: node.suspicions?.length || 0,
        img: nodeTypeIcon[node.type] || '/img/default.svg',
      }
      return {
        id: node.id,
        label: wrapLine(node.id, 17),
        nodeData: nodeData,
      }
    })
  }

  if (data.links) {
    graphData.edges = data.links.map(link => {
      return {
        id: link.id,
        source: link.source,
        target: link.destination,
        label: link.type
      }
    })
  }

  return graphData
}

const bindEvent = (graph: Graph, onClick: ((type: "node" | "edge", id: string) => void) | undefined) => {
  graph.on('node:mouseenter', (e) => {
    const nodeItem = e.item;
    graph.setItemState(nodeItem!, 'hover', true);
  });

  graph.on('node:mouseleave', (e) => {
    const nodeItem = e.item;
    graph.setItemState(nodeItem!, 'hover', false);
  });

  graph.on('edge:mouseenter', (e) => {
    const edgeItem = e.item;
    graph.setItemState(edgeItem!, 'hover', true);
  });

  graph.on('edge:mouseleave', (e) => {
    const edgeItem = e.item;
    graph.setItemState(edgeItem!, 'hover', false);
  });

  if (onClick) {
    graph.on('node:click', (e) => {
      const nodeItem = e.item
      onClick('node', e.item?._cfg?.id!);
    })

    graph.on('edge:click', (e) => {
      const nodeItem = e.item
      onClick('edge', e.item?._cfg?.id!);
    })
  }
}

const DiagnosisResult: React.FC<DiagnosisResultProps> = (props: DiagnosisResultProps): JSX.Element => {
  const ref = useRef<HTMLDivElement>(null)
  let graph: Graph | null = null;

  registerDiagnosisNode()

  const { data, onClick } = props
  const graphData = data ? toGraphData(data) : null
  useEffect(() => {
    if (!data || !graphData) return;
    graph = new Graph({
      container: ref.current!,
      width: ref.current?.clientWidth,
      height: ref.current?.clientHeight,
      fitCenter: true,
      fitView: true,
      fitViewPadding: [0, 100],
      layout: {
        type: 'dagre',
        rankdir: "LR"
      },
      defaultNode: {
        type: "diagnosis-node",
        size: 160,
        labelCfg: {
          style: {
            lineWidth: 1,
            fontSize: 16,
            cursor: 'pointer'
          }
        },
        style: {
          stroke: 'black',
          cursor: 'pointer',
          fill: "#ffffff",
          fillOpacity: 0,
          lineWidth: 1.7
        }
      },
      nodeStateStyles: {
        hover: {
          lineWidth: 2.5
        }
      },
      edgeStateStyles: {
        hover: {
          lineWidth: 2.5
        }
      },
      defaultEdge: {
        labelCfg: {
          refY: 3,
          lineWidth: 1,
          style: {
            cursor: 'pointer',
            textBaseline: 'bottom',
            fontSize: 17
          }
        },
        style: {
          cursor: 'pointer',
          stroke: 'black',
          endArrow: {
            path: 'M 0,0 L 10,5 L 10,-5 Z',
            fill: 'black',
            stroke: 'black'
          },
          lineWidth: 1.7
        },
      }
    });

    graph.data(graphData);
    graph.render();
    bindEvent(graph, onClick)

    const graphResize = (entries: ResizeObserverEntry[]) => {
      if (graph) {
        if (!entries || entries.length === 0) return;
        const e = entries[0]
        console.log(e)
        graph.changeSize(e.target.clientWidth, e.target.clientHeight);
        graph.fitView()
      }
    }

    const ob = new ResizeObserver(graphResize)
    ob.observe(ref.current!);
    return () => { graph?.destroy(); ob.disconnect() }
  }, [data])

  return (
    <div ref={ref} style={{ height: '80vh' }}>
    </div>
  );
};

export default DiagnosisResult;
