import { Form, Input, Select, Radio, Checkbox, TimePicker, Dialog, List, Divider, Box } from '@alifd/next';
import React, { useEffect, useState } from "react";
import moment from 'moment';
import k8sService, { NodeInfo, PodInfo } from "@/services/k8s";
import styles from "./index.module.css"


interface SelectorProps {
  submitSelector: (data: []) => void;
  podList: PodInfo[];
  nodeList: NodeInfo[];
  visible: boolean;
  displayIPSelector: boolean
  onClose: () => void
}

const SelectorDialog: React.FunctionComponent<SelectorProps> = (props: SelectorProps) => {
  const [nameList, setNameList] = useState([])
  const [namespaces, setNamespaces] = useState([])
  const [formName, setFormName] = useState("")
  const [formCaptureType, setFormCaptureType] = useState("Pod")
  const [captureSelectorType, setCaptureSelectorType] = useState("Name")
  const [labelSelectorKeys, setLabelSelectorKeys] = useState([])
  const [labelSelectorKey, setLabelSelectorKey] = useState("")
  const [labelSelectorValues, setLabelSelectorValues] = useState([])
  const [labelSelectorValue, setLabelSelectorValue] = useState("")
  const [formNamespace, setFormNamespace] = useState("")
  const [ipAddress, setIPAddress] = useState("")
  const [ipAddressCheckState, setIPAddressCheckState] = useState("error")

  const filterCaptureObject = (type, ns) => {
    if (type == "Node") {
      k8sService.listNodes().then((res) => {
        setNameList(res)
      })
    } else {
      k8sService.listPods().then((res) => {
        setNamespaces([...new Set(res.map(item => item.namespace))].map(item => ({ name: item })))
        setNameList(res.filter(item => item.namespace == ns))
      })
    }
  }

  useEffect(() => {
    filterCaptureObject(formCaptureType, formNamespace)
  }, []);

  useEffect(() => {
    if (captureSelectorType == "Name") {
      setLabelSelectorKeys([])
    } else {
      let labelKeys = []
      nameList.forEach((item, idx, list) => {
        if (item.labels) {
          labelKeys = [...labelKeys, ...Object.keys(item.labels)]
        }
      })

      setLabelSelectorKeys([...new Set(labelKeys)])
    }
    setLabelSelectorKey("")
  }, [formCaptureType, namespaces, captureSelectorType, formNamespace, nameList]);
  useEffect(() => {
    setLabelSelectorValue("")
    if (captureSelectorType == "Name") {
      setLabelSelectorValues([])
    } else {
      let values = nameList.map(item => {
        if (formCaptureType == "Pod") {
          if (item.namespace && item.namespace == formNamespace) {
            if (item.labels) {
              if (item.labels[labelSelectorKey]) {
                return item.labels[labelSelectorKey]
              }
            }
          }
        } else {
          if (item.labels) {
            if (item.labels[labelSelectorKey]) {
              return item.labels[labelSelectorKey]
            }
          }
        }
      })
      values = [...new Set(values)]
      setLabelSelectorValues(values)
    }
  }, [labelSelectorKeys, labelSelectorKey]);


  const selectedResult = () => {
    if (formCaptureType == "Pod") {
      if (captureSelectorType == "Name") {
        let capList = [
          {
            name: formName.name,
            namespace: formNamespace,
            nodename: formName.nodename,
            type: "Pod"
          }
        ]
        return capList
      } else {
        let selectPod = nameList.map((item) => {
          if (item.namespace && item.namespace == formNamespace && item.labels[labelSelectorKey] && item.labels[labelSelectorKey] == labelSelectorValue) {
            item.type = "Pod"
            return item
          }
        })
        selectPod = [...new Set(selectPod.filter(item => item))]
        return selectPod
      }
    } else if (formCaptureType == "Node") {
      if (captureSelectorType == "Name") {
        return [
          {
            name: formName.name,
            type: "Node"
          }
        ]
      } else {
        return [...new Set(nameList.map((item) => {
          if (item.labels[labelSelectorKey] && item.labels[labelSelectorKey] == labelSelectorValue) {
            item.type = "Node"
            return item
          }
        }).filter(item => item))]
      }
    } else if (formCaptureType == "IP") {
      setFormCaptureType("Pod")
      return [
        {
          name: ipAddress,
          type: "IP"
        }
      ]
    }
    return []
  }

  const isPodOrNode = (type) => {
    return type == "Pod" || type == "Node"
  }

  return (
    <Dialog
      v2
      title="Add Target"
      footerActions={['ok']}
      visible={props.visible}
      onClose={props.onClose}
      onOk={() => {!(formCaptureType=="IP"&&ipAddressCheckState!="success") && props.submitSelector(selectedResult())}}
    >
      <Form
        labelAlign='left'
        labelCol={{ fixedSpan: 6 }}
        wrapperCol={{ span: 16 }}
      >
        <Form.Item label="Type">
          <Radio.Group
            shape="button"
            value={formCaptureType}
            onChange={(value) => {setFormCaptureType(value); setFormNamespace(""); setFormName(""); filterCaptureObject(value) }}
          >
            <Radio value="Node">Node</Radio>
            <Radio value="Pod">Pod</Radio>
            {props.displayIPSelector &&
              <Radio value="IP">IP</Radio>
            }
          </Radio.Group>
        </Form.Item>
        {isPodOrNode(formCaptureType)  &&
          <Form.Item label="Select Target By">
            <Radio.Group
              shape="button"
              value={captureSelectorType}
              onChange={(value) => { setCaptureSelectorType(value); setFormNamespace(""); setFormName(""); filterCaptureObject(formCaptureType) }}
            >
              <Radio value="Name">Namespace & Name</Radio>
              <Radio value="Selector">Label Selector</Radio>
            </Radio.Group>
          </Form.Item>
        }
        {!isPodOrNode(formCaptureType) &&
          <Form.Item label="IP Address">
            <Input onChange={(value) => {setIPAddressCheckState(/^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$/.test(value) ? "success" : "error");setIPAddress(value)}}
              state={ipAddressCheckState}
              placeholder="Input IP Address, eg: 1.1.1.1"
            />
          </Form.Item>
        }

        {formCaptureType == "Pod" &&
          <Form.Item label="Namespace" required >
            <Select className={styles.selector} name="namespace" placeholder="Please select namespace" dataSource={namespaces} useDetailValue showSearch
              onChange={function (value) { setFormNamespace(value.name); setFormName(""); filterCaptureObject(formCaptureType, value.name); }}
              itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`}
              filter={(k, i) => i.name.startsWith(k)}
            />
          </Form.Item>
        }

        {isPodOrNode(formCaptureType)  && captureSelectorType == "Selector" &&
          <Form.Item label="LabelSelector" required>
            <Box direction="row" style={{alignItems: "center"}}>
              <Select showSearch value={labelSelectorKey} onChange={setLabelSelectorKey} dataSource={labelSelectorKeys} style={{ width: 200 }} name="labelKey" placeholder="key" />
              <span style={{margin: "0 10px"}}>=</span>
              <Select showSearch value={labelSelectorValue} onChange={setLabelSelectorValue} dataSource={labelSelectorValues} style={{ width: 200 }} name="labelVal" placeholder="value" />
            </Box>
          </Form.Item>
        }
        {isPodOrNode(formCaptureType) && captureSelectorType == "Name" &&
          <Form.Item label="Name" required>
            <Select className={styles.selector} name="name" placeholder="Please select name" useDetailValue showSearch
              value={formName}
              dataSource={nameList}
              itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`}
              onChange={(value) => { setFormName(value) }}
              filter={(k, i) => i.name.startsWith(k)}
            />
          </Form.Item>
        }
      </Form>
    </Dialog>
  );
}

export default SelectorDialog;
