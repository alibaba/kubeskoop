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
  const [showSelectorDialog, setShowSelectorDialog] = useState(false)

  const [pingMeshList, setPingMeshList] = useState([])
  const handleSubmit = (values: PingMeshArgs, errors: any) => {
    if (errors) {
      return
    }
    if (pingMeshList.length < 2) {
      Message.error("You have to select at least two targets.")
      return
    }
    values.ping_mesh_list = pingMeshList
    onSubmit(values);
  };

  return (
    <Form inline labelAlign='left'>
      <Form.Item label="Targets" >
        <div className={styles.custom}>
          {pingMeshList.map((v, i) => {
            if (v.type == "Node") {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
            } else {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.namespace + "/" + v.name}</Button>;
            }
          })}
          <Button className={styles.btn} type="primary" onClick={() => { setShowSelectorDialog(!showSelectorDialog) }}>
            Add ＋{" "}
          </Button>
          <Button className={styles.btn} warning type="primary" onClick={() => { setPingMeshList([]) }}>
            Clear
          </Button>
          <SelectorDialog visible={showSelectorDialog}
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
