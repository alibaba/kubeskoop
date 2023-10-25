import { Form, Input, NumberPicker, Select } from '@alifd/next';

interface DiagnosisFormProps {
  onSubmit: (data: DiagnosisFormData) => void;
}

interface DiagnosisFormData {
  [key: string]: any;
}

const ipRegex = /^(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])\.(\d{1,2}|1\d\d|2[0-4]\d|25[0-5])$/;

const DiagnosisForm: React.FunctionComponent<DiagnosisFormProps> = (props: DiagnosisFormProps) => {
  const { onSubmit } = props;
  const handleSubmit = (values: DiagnosisFormData, errors: any) => {
    if (errors) {
      return;
    }
    onSubmit(values);
  };
  return (
    <Form inline labelAlign='left'>
      <Form.Item label="源地址" required patternMessage="请输入正确的IP地址" pattern={ipRegex}>
        <Input name="src" placeholder="请输入源IP地址" />
      </Form.Item>
      <Form.Item label="目的地址" required patternMessage='请输入正确的IP地址' pattern={ipRegex}>
        <Input name="dst" placeholder="请输入目的IP地址" />
      </Form.Item>
      <Form.Item label="端口" required>
        <NumberPicker name="port" min={1} max={65535} hasTrigger={false}/>
      </Form.Item>
      <Form.Item label="协议" required>
        <Select name="protocol">
          <Select.Option value="tcp">TCP</Select.Option>
          <Select.Option value="udp">UDP</Select.Option>
        </Select>
      </Form.Item>
      <Form.Item>
        <Form.Submit type="primary" validate onClick={handleSubmit}>
          发起诊断
        </Form.Submit>
      </Form.Item>
    </Form>
  );
};

export default DiagnosisForm;
