import React, { useState, useEffect } from 'react';
import { Modal, Form, Select, InputNumber, message, Alert, AutoComplete } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { getConfig, createPod } from '../../services/api';
import './CreatePodModal.css';

interface CreatePodModalProps {
  visible: boolean;
  onCancel: () => void;
  onSuccess: () => void;
  currentQuota: any;
}

const CreatePodModal: React.FC<CreatePodModalProps> = ({
  visible,
  onCancel,
  onSuccess,
  currentQuota,
}) => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<any>(null);
  const [selectedGPUCount, setSelectedGPUCount] = useState(1);

  useEffect(() => {
    if (visible) {
      loadConfig();
      form.resetFields();
      setSelectedGPUCount(1);
    }
  }, [visible]);

  const loadConfig = async () => {
    try {
      const data: any = await getConfig();
      setConfig(data);
      if (data.presetImages && data.presetImages.length > 0) {
        form.setFieldsValue({ image: data.presetImages[0].image });
      }
      if (data.gpuTypes && data.gpuTypes.length > 0) {
        form.setFieldsValue({ gpuType: data.gpuTypes[0].name });
      }
      form.setFieldsValue({
        gpuCount: 1,
        cpu: data.ui?.defaultCPU || '4',
        memory: data.ui?.defaultMemory || '8Gi',
      });
    } catch (error: any) {
      message.error(`加载配置失败: ${error.message}`);
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      const payload = { ...values };
      if (payload.gpuCount === 0) {
        delete payload.gpuType;
      }
      await createPod(payload);
      onSuccess();
    } catch (error: any) {
      if (error.errorFields) return;
      message.error(`创建失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const willExceedQuota = () => {
    const newPodCount = currentQuota.podUsed + 1;
    const newGPUCount = currentQuota.gpuUsed + selectedGPUCount;
    return {
      podExceeded: newPodCount > currentQuota.podLimit,
      gpuExceeded: newGPUCount > currentQuota.gpuLimit,
      newPodCount,
      newGPUCount,
    };
  };

  const quotaCheck = willExceedQuota();
  const canCreate = !quotaCheck.podExceeded && !quotaCheck.gpuExceeded;

  return (
    <Modal
      title={
        <div className="modal-title-custom">
          <PlusOutlined />
          <span>创建新的 Pod</span>
        </div>
      }
      open={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
      okText="创建 Pod"
      cancelText="取消"
      okButtonProps={{ disabled: !canCreate }}
      width={600}
      className="create-pod-modal"
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{ gpuCount: 1, cpu: '4', memory: '8Gi' }}
        className="create-pod-form"
      >
        <Form.Item
          label="基础镜像"
          name="image"
          rules={[
            { required: true, message: '请输入镜像' },
            { pattern: /^[a-zA-Z0-9\-_./:]+$/, message: '请输入有效的镜像名称' },
          ]}
          help={config?.ui?.enableCustomImage ? '可以从列表选择或输入自定义镜像' : '请从预设列表中选择镜像'}
        >
          {config?.ui?.enableCustomImage ? (
            <AutoComplete
              placeholder="选择或输入镜像名称"
              filterOption={(inputValue, option) =>
                option?.value ? String(option.value).toUpperCase().indexOf(inputValue.toUpperCase()) !== -1 : false
              }
              options={config?.presetImages?.map((img: any) => ({
                value: img.image,
                label: `${img.name} - ${img.description}`,
              }))}
            />
          ) : (
            <Select placeholder="选择镜像" showSearch optionFilterProp="children">
              {config?.presetImages?.map((img: any) => (
                <Select.Option key={img.image} value={img.image}>
                  {img.name} - {img.description}
                </Select.Option>
              ))}
            </Select>
          )}
        </Form.Item>

        <div className="form-row">
          <Form.Item
            label="CPU 核数"
            name="cpu"
            rules={[
              { required: true, message: '请输入 CPU 核数' },
              { pattern: /^[0-9]+(\.[0-9]+)?$/, message: '请输入有效的数字' },
            ]}
            className="form-col"
          >
            <AutoComplete
              placeholder="选择或输入"
              options={(config?.ui?.cpuOptions || ['2', '4', '8', '16']).map((cpu: string) => ({
                value: cpu,
                label: `${cpu} 核`,
              }))}
            />
          </Form.Item>

          <Form.Item
            label="内存大小"
            name="memory"
            rules={[
              { required: true, message: '请输入内存大小' },
              { pattern: /^[0-9]+(\.[0-9]+)?(Mi|Gi)$/, message: '格式如 4Gi, 512Mi' },
            ]}
            className="form-col"
          >
            <AutoComplete
              placeholder="选择或输入"
              options={(config?.ui?.memoryOptions || ['4Gi', '8Gi', '16Gi', '32Gi']).map((mem: string) => ({
                value: mem,
                label: mem,
              }))}
            />
          </Form.Item>
        </div>

        <div className="form-row">
          <Form.Item
            label="GPU 数量"
            name="gpuCount"
            rules={[{ required: true, message: '请选择 GPU 数量' }]}
            help="设置为 0 可创建纯 CPU Pod"
            className="form-col"
          >
            <InputNumber
              min={0}
              max={8}
              style={{ width: '100%' }}
              onChange={(value) => setSelectedGPUCount(value || 0)}
            />
          </Form.Item>

          <Form.Item
            label="GPU 类型"
            name="gpuType"
            rules={[{ required: selectedGPUCount > 0, message: '请选择 GPU 类型' }]}
            hidden={selectedGPUCount === 0}
            className="form-col"
          >
            <Select placeholder="选择 GPU 类型" allowClear>
              {config?.gpuTypes?.map((gpu: any) => (
                <Select.Option key={gpu.name} value={gpu.name}>{gpu.name}</Select.Option>
              ))}
            </Select>
          </Form.Item>
        </div>

        <div className="quota-preview">
          <div className="quota-preview-title">创建后配额使用</div>
          <div className="quota-preview-items">
            <div className={`quota-preview-item ${quotaCheck.podExceeded ? 'exceeded' : ''}`}>
              <span className="quota-label">Pod</span>
              <span className="quota-value">{quotaCheck.newPodCount} / {currentQuota.podLimit}</span>
            </div>
            <div className={`quota-preview-item ${quotaCheck.gpuExceeded ? 'exceeded' : ''}`}>
              <span className="quota-label">GPU</span>
              <span className="quota-value">{quotaCheck.newGPUCount} / {currentQuota.gpuLimit}</span>
            </div>
          </div>
        </div>

        <Alert
          message={<span style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span>⏰</span> 所有 Pod 将在今晚 23:00 自动删除</span>}
          type="warning"
          showIcon={false}
          className="time-warning"
        />
      </Form>
    </Modal>
  );
};

export default CreatePodModal;
