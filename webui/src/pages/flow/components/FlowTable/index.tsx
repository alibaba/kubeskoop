import { Table, Pagination } from "@alifd/next"
import { useEffect, useMemo, useState } from "react"
import styles from './index.module.css'

interface FlowTableProps {
  data: any
}

const columns = [
  {
    title: '源',
    dataIndex: 'srcName',
  },
  {
    title: '源地址',
    dataIndex: 'srcAddr',
  },
  {
    title: '目的',
    dataIndex: 'dstName',
  },
  {
    title: '目的地址',
    dataIndex: 'dstAddr',
  },
    {
    title: '协议',
    dataIndex: 'protocol',
  },
  {
    title: '字节数',
    dataIndex: 'bytes',
    sortable: true,
    sortDirections: ["desc", "asc"],
  },
  {
    title: '数据包数',
    dataIndex: 'packets',
    sortable: true,
    sortDirections: ["desc", "asc"],
  },
];

const getName = (node: any) => {
  if (!node) {
    return 'unknown'
  }
  switch (node.type) {
    case 'node':
      return node.node_name;
    case 'pod':
      return `${node.namespace}/${node.name}`;
    case 'external':
      return node.ip;
    default:
      return node.id || 'unknown';
  }
}

const toData = (data: any, nodeMap: any) => {
  return data.edges.map(item => {
    return {
      srcName: getName(nodeMap[item.src]),
      dstName: getName(nodeMap[item.dst]),
      srcAddr: item.sport != 0 ? `${item.src}:${item.sport}` : item.src,
      dstAddr: item.dport!= 0 ? `${item.dst}:${item.dport}` : item.dst,
      ...item
    }
  })
}

const toPagedData = (data: any[], page: number, size: number): any[] => {
  const start = (page - 1) * size;
  const end = page * size;
  return data.slice(start, end);
}

const FlowTable: React.FC<FlowTableProps> = (props: FlowTableProps): JSX.Element => {
  const [currentPage, setCurrentPage] = useState(1);
  const onPageChange = (page: number) => {
    setCurrentPage(page);
  }

  const pageSize = 100;

  const [data, setData] = useState([]);
  useEffect(() => {
    const nodeMap = {};
    props.data.nodes.forEach(item => {
      nodeMap[item.id] = item;
    })

    setData(toData(props.data, nodeMap));
  }, [props.data]);

  const onSort = (dataIndex: string, order: string) => {
    console.log(`onSort: ${dataIndex}, ${order}`)
    const sortedData = data.toSorted((a, b) => {
      return order === 'asc' ? a[dataIndex] - b[dataIndex] : b[dataIndex] - a[dataIndex];
    });
    setData(sortedData);
    setCurrentPage(1);
  }

  return (
    <div>
      <Table.StickyLock
        className={styles.tableCell}
        columns={columns}
        dataSource={toPagedData(data, currentPage, pageSize)}
        isZebra
        hasBorder={false}
        fixedHeader
        maxBodyHeight="70vh"
        onSort={onSort}
      />
      <Pagination
        type="simple"
        current={currentPage}
        pageSize={pageSize}
        total={data.length}
        totalRender={total => `总数：${total}`}
        style={{ marginTop: 16, textAlign: 'right' }}
        onChange={onPageChange}
      />
    </div>
  );
};

export default FlowTable;
