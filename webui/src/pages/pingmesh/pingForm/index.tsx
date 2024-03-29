import { Form, Button, Message } from '@alifd/next';
import { useState } from "react";
import styles from "./index.module.css"
import SelectorDialog from "./selectorDialog.tsx"
import { PingMeshArgs } from "@/services/pingmesh";

interface PingFormProps {
  onSubmit: (data: PingMeshArgs) => void;
}

const PingForm: React.FunctionComponent<PingFormProps> = (props: PingFormProps) => {
  const { onSubmit } = props;
  const [showSourceSelectorDialog, setShowSourceSelectorDialog] = useState(false)
  const [showSelectorDialog, setShowSelectorDialog] = useState(false)

  const [pingMeshSourceList, setPingMeshSourceList] = useState([])
  const [pingMeshList, setPingMeshList] = useState([])
  const handleSubmit = (values: PingMeshArgs, errors: any) => {
    if (errors) {
      return
    }
    if (pingMeshList.length == 0 || pingMeshSourceList.length == 0) {
      Message.error("You have to select src and dst object to detect the latency")
      return
    }
    values.ping_mesh_source_list = pingMeshSourceList
    values.ping_mesh_list = pingMeshList
    onSubmit(values);
  };

  return (
    <Form inline labelAlign='left'>
      <Form.Item>
        <div className={styles.custom}>
          <span className={styles.directionSel}>Source: </span>
          <div className={styles.selectorGroup}>
          <Button className={styles.btn} type="primary" onClick={() => { setShowSourceSelectorDialog(!showSourceSelectorDialog) }}>
            Add ＋{" "}
          </Button>
          <Button className={styles.btn} warning type="primary" onClick={() => { setPingMeshSourceList([]) }}>
            Clear
          </Button>
          <div>
            {pingMeshSourceList.map((v, i) => {
              if (v.type == "Node" || v.type == "IP") {
                return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
              } else {
                return <Button className={styles.btn} key={i}>{v.type + ": " + v.namespace + "/" + v.name}</Button>;
              }
            })}
          </div>

          <SelectorDialog visible={showSourceSelectorDialog}
            submitSelector={(value) => {
              let toAdd = []
              skip: for (const v of value.values()) {
                for (const c of pingMeshSourceList.values()) {
                  if (v.name == c.name) {
                    continue skip
                  }
                }
                toAdd = [...toAdd, v]
              }
              setPingMeshSourceList([...pingMeshSourceList, ...toAdd])
              setShowSourceSelectorDialog(!showSourceSelectorDialog)
            }}
            onClose={() => { setShowSourceSelectorDialog(!showSourceSelectorDialog) }}></SelectorDialog>
          </div>
        </div>
      </Form.Item>
      <br/>
      <Form.Item>
        <span className={styles.directionSel}>Destination: </span>
        <div className={styles.custom}>
          <div className={styles.selectorGroup}>
            <Button className={styles.btn} type="primary" onClick={() => { setShowSelectorDialog(!showSelectorDialog) }}>
              Add ＋{" "}
            </Button>
            <Button className={styles.btn} warning type="primary" onClick={() => { setPingMeshList([]) }}>
              Clear
            </Button>
            <br/>
          {pingMeshList.map((v, i) => {
            if (v.type == "Node" || v.type == "IP") {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
            } else {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.namespace + "/" + v.name}</Button>;
            }
          })}

          <SelectorDialog visible={showSelectorDialog} displayIPSelector={true}
                          submitSelector={(value) => {
                            let toAdd = []
                            skip: for (const v of value.values()) {
                              for (const c of pingMeshList.values()) {
                                if (v.name == c.name) {
                                  continue skip
                                }
                              }
                              toAdd = [...toAdd, v]
                            }
                            setPingMeshList([...pingMeshList, ...toAdd])
                            setShowSelectorDialog(!showSelectorDialog)
                          }}
                          onClose={() => { setShowSelectorDialog(!showSelectorDialog) }}></SelectorDialog>
          </div>
          </div>
      </Form.Item>
      <br />
      <Form.Item>
        <Form.Submit type="primary" validate onClick={handleSubmit}>
          Submit
        </Form.Submit>
      </Form.Item>
    </Form>
  );
};

export default PingForm;
