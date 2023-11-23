import PageHeader from '@/components/PageHeader';
import FlowGraph from './components/FlowGraph'
import FlowTable from './components/FlowTable'
import { useEffect, useMemo, useState } from 'react';
import { Card, Select, Radio, Switch, Box, Message, Button, Icon, Loading } from '@alifd/next';
import styles from './index.module.css';
import flowService, { FlowData } from '@/services/flow'
import { getErrorMessage } from '@/utils';

const getNamespaces = (data: any) => {
  console.log('getNamespaces')
  return data.nodes.map((node: any) => {
    return node.namespace || 'default'
  }).filter((item, index, arr) => {
    return arr.indexOf(item) === index
  })
}

const filterFlowData = (data: FlowData, namespaces: string[], nodes: string[], showExternal: boolean) => {
  const filteredNode = data.nodes.filter((node: any) => {
    return !namespaces || node.type != 'pod' || namespaces.includes(node.namespace)
  })
    .filter((node: any) => {
      return !nodes || node.type == 'external' || nodes.includes(node.nodeName)
    })
    .filter((node: any) => {
      return showExternal || node.type !== 'external'
    });

  const nodeMap = {};
  filteredNode.forEach(i => {
    nodeMap[i.id] = true
  });

  const filteredEdge = data.edges.filter((edge: any) => {
    return nodeMap[edge.src] && nodeMap[edge.dst]
  });

  filteredNode.sort((a, b) => {
    return a.id.localeCompare(b.id)
  })

  filteredEdge.sort((a, b) => {
    return a.id.localeCompare(b.id)
  })

  return {
    nodes: filteredNode,
    edges: filteredEdge,
  }
}

export default function Dashboard() {
  const [data, setData] = useState({ nodes: [], edges: [] });
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const getFlowData = (initial: boolean) => {
    setLoading(true);
    flowService.getFlowData().then((res) => {
      if (initial) setSelectedNamespaces(getNamespaces(res));
      setData(res);
      setLoading(false);
    }).catch(err => {
      Message.error(`数据获取失败：${getErrorMessage(err)}`)
    });
  };
  useEffect(() => getFlowData(true), []);

  const [viewMode, setViewMode] = useState('graph');
  const onViewModeChange = (value: string) => {
    setViewMode(value);
  }

  const namespaces = useMemo(() => getNamespaces(data), [data]);
  const [showExternal, setShowExternal] = useState(false);

  const onNamespacesChange = (value: string[]) => {
    console.log(value);
    setSelectedNamespaces(value);
  }

  const onShowExternalChange = (checked: boolean) => {
    setShowExternal(checked);
  }

  const filtered = useMemo(
    () => filterFlowData(data, selectedNamespaces, null, showExternal),
    [data, selectedNamespaces, showExternal]
  )

  const view = () => {
    switch (viewMode) {
      case 'graph':
        return <FlowGraph data={filtered} />
      case 'table':
        return <FlowTable data={filtered} />
    };
    return null;
  }


  return (
    <div>
      <PageHeader
        title='链路图'
        breadcrumbs={[{ name: 'Console' }, { name: '主页' }]}
      />
      <Card contentHeight="auto">
        <Card.Content style={{ paddingLeft: 0 }}>
          <Box className={styles.contentBox} direction='row'>
            <span className={styles.optionLabel}>命名空间</span>
            <Select
              name="namespace"
              dataSource={namespaces}
              mode='multiple'
              onChange={onNamespacesChange}
              tagInline
              hasSelectAll
              showSearch
              hasClear
              useVirtual
              style={{ width: '100%', maxWidth: 300 }}
            />
          </Box>
          <Box direction='row'>
            <span className={styles.optionLabel}>显示集群外端点</span>
            <Switch id='showExternal' style={{ marginRight: '10px' }} onChange={onShowExternalChange} />
            <span className={styles.optionLabel}>浏览模式</span>
            <Radio.Group shape='button' defaultValue='graph' style={{ marginRight: '10px' }} onChange={onViewModeChange}>
              <Radio id='graph' value='graph'>Graph</Radio>
              <Radio id='table' value='table'>Table</Radio>
            </Radio.Group>
            <Button
              type="secondary"
              size="medium"
              style={{ marginLeft: 'auto', padding: "0 13px" }}
              onClick={() => getFlowData(false)}
            >
              <Icon type="refresh" />
              <span>刷新</span>
            </Button>
          </Box>
        </Card.Content>
        <Card.Divider />
        <Card.Content style={{ padding: 0 }}>
          <Loading visible={loading} inline={false}>
            {view()}
          </Loading>
        </Card.Content>
      </Card>
    </div>
  );
}
