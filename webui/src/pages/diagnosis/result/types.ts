
const enum SuspicionLevel {
  INFO = 0,
  WARNING = 1,
  CRITICAL = 2,
  FATAL = 3
};

interface DiagnosisNodeAction {
  type: string
};

interface Suspicion {
  level: SuspicionLevel
  message: string
};

interface DiagnosisCluster {
  suspicions: Suspicion
};

interface DiagnosisPacket {
  source: string
  destination: string
  dport: number
  protocol: string
};

interface DiagnosisNode {
  id: string
  type: string
  suspicions: Suspicion[]
  actions: Map<string, DiagnosisNodeAction>
};

interface DiagnosisLink {
  id: string,
  type: string
  action: string
  source: string,
  source_attributes: any
  destination: string
  destination_attributes: any
  packet: DiagnosisPacket
};

interface DiagnosisResultData {
  cluster: DiagnosisCluster
  nodes: DiagnosisNode[]
  links: DiagnosisLink[]
};
