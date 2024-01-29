import FlowDashboard from "../monitoring/flow";
import { definePageConfig } from "ice";

export default FlowDashboard;
export const pageConfig = definePageConfig(() => {
  return {
    title: 'Network Graph',
  };
});
