const asideMenuConfig = [
  {
    name: "主页",
    path: "/",
  },
  {
    name: '监控',
    children: [
      {
        name: "大盘",
        path: "/monitoring/dashboard"
      },
      {
        name: "事件",
        path: "/monitoring/events"
      }
      ]
  },
  {
    name: '诊断',
    children: [
      {
        name: "连通性诊断",
        path: "/diagnosis",
      },
      {
        name: "抓包",
        path: "/capture"
      },
      {
        name: "延迟探测(PingMesh)",
        path: "/pingmesh"
      }
    ]
  },
  {
    name: '配置',
    children: [
      {
        name: '控制台配置',
        path: '/config',
      },
      {
        name: '节点配置',
        path: '/node_config',
      }
    ],
  },
];

export { asideMenuConfig };
