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
      <Form.Item label="Source Address" required patternMessage='Please enter a valid IP address.' pattern={ipRegex}>
        <Input name="src" placeholder="The source IP"/>
      </Form.Item>
      <Form.Item label="Destination Address" required patternMessage='Please enter a valid IP address.' pattern={ipRegex}>
        <Input name="dst" placeholder="The destination IP" />
      </Form.Item>
      <Form.Item label="Port" required>
        <NumberPicker name="port" min={1} max={65535} hasTrigger={false}/>
      </Form.Item>
      <Form.Item label="Protocol" required>
        <Select name="protocol">
          <Select.Option value="tcp">TCP</Select.Option>
          <Select.Option value="udp">UDP</Select.Option>
        </Select>
      </Form.Item>
      <Form.Item>
        <Form.Submit type="primary" validate onClick={handleSubmit}>
          Diagnose
        </Form.Submit>
      </Form.Item>
    </Form>
  );
};

export default DiagnosisForm;
