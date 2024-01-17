const asideMenuConfig = [
  {
    name: "Network Graph",
    path: "/",
  },
  {
    name: 'Monitoring',
    children: [
      {
        name: "Dashboard",
        path: "/monitoring/dashboard"
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
