const asideMenuConfig = [
  {
    name: "主页",
    path: "/",
  },
  {
    name: '监控',
    children: [
      {
        name: "事件",
        path: "/monitoring/events"
      },
      {
        name: "flow",
        path: "/monitoring/flow"
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
      }
    ]
  },
  {
    name: '配置',
    children: [
      {
        name: '节点配置',
        path: '/configuration',
      }
    ],
  },
];

export { asideMenuConfig };
