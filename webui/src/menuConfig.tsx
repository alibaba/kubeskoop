const asideMenuConfig = [
  {
    name: "Network Graph",
    path: "/",
  },
  {
    name: 'Monitoring',
    children: [
      {
        name: "Node Dashboard",
        path: "/monitoring/dashboard/node"
      },
      {
        name: "Pod Dashboard",
        path: "/monitoring/dashboard/pod"
      },
      {
        name: "Events",
        path: "/monitoring/events"
      },
      {
        name: "Network Graph",
        path: "/monitoring/flow"
      }
    ]
  },
  {
    name: 'Diagnosis',
    children: [
      {
        name: "Connectivity Diagnosis",
        path: "/diagnosis",
      },
      {
        name: "Packet Capturing",
        path: "/capture"
      },
      {
        name: "Latency Detection",
        path: "/pingmesh"
      }
    ]
  },
  {
    name: 'Configuration',
    children: [
      {
        name: 'Node Configuration',
        path: '/config',
      }
    ],
  },
];
export { asideMenuConfig };
