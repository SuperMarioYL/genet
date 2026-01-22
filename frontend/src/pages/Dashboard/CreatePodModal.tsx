import React, { useState, useEffect } from 'react';
import { Modal, Form, Select, InputNumber, message, Alert, AutoComplete } from 'antd';
import { getConfig, createPod } from '../../services/api';

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
      setSelectedGPUCount(1); // 默认为 1
    }
  }, [visible]);

  const loadConfig = async () => {
    try {
      const data: any = await getConfig();
      setConfig(data);
      // 设置默认值
      if (data.presetImages && data.presetImages.length > 0) {
        form.setFieldsValue({ image: data.presetImages[0].image });
      }
      if (data.gpuTypes && data.gpuTypes.length > 0) {
        form.setFieldsValue({ gpuType: data.gpuTypes[0].name });
      }
      form.setFieldsValue({
        gpuCount: 1, // 默认为 1，可以改为 0
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

      // 如果 GPU 数量为 0，删除 gpuType 字段
      const payload = { ...values };
      if (payload.gpuCount === 0) {
        delete payload.gpuType;
      }

      await createPod(payload);
      onSuccess();
    } catch (error: any) {
      if (error.errorFields) {
        // 表单验证错误
        return;
      }
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
      title="创建新的 GPU Pod"
      open={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
      okText="创建 Pod"
      cancelText="取消"
      okButtonProps={{ disabled: !canCreate }}
      width={600}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          gpuCount: 1,
          cpu: '4',
          memory: '8Gi',
        }}
      >
        <Form.Item
          label="基础镜像"
          name="image"
          rules={[
            { required: true, message: '请输入镜像' },
            {
              pattern: /^[a-zA-Z0-9\-_./:]+$/,
              message: '请输入有效的镜像名称',
            },
          ]}
          help={
            config?.ui?.enableCustomImage
              ? '可以从列表选择或输入自定义镜像（如 ubuntu:22.04）'
              : '请从预设列表中选择镜像'
          }
        >
          {config?.ui?.enableCustomImage ? (
            <AutoComplete
              placeholder="选择或输入镜像名称"
              filterOption={(inputValue, option) =>
                option?.value
                  ? String(option.value).toUpperCase().indexOf(inputValue.toUpperCase()) !== -1
                  : false
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

        <Form.Item
          label="GPU 数量"
          name="gpuCount"
          rules={[{ required: true, message: '请选择 GPU 数量' }]}
          help="设置为 0 可创建纯 CPU Pod"
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
          rules={[
            {
              required: selectedGPUCount > 0,
              message: '使用 GPU 时请选择 GPU 类型',
            },
          ]}
          hidden={selectedGPUCount === 0}
        >
          <Select placeholder="选择 GPU 类型" allowClear>
            {config?.gpuTypes?.map((gpu: any) => (
              <Select.Option key={gpu.name} value={gpu.name}>
                {gpu.name}
              </Select.Option>
            ))}
          </Select>
        </Form.Item>

        <Form.Item
          label="CPU 核数"
          name="cpu"
          rules={[
            { required: true, message: '请输入 CPU 核数' },
            {
              pattern: /^[0-9]+(\.[0-9]+)?$/,
              message: '请输入有效的数字（如 2, 4, 0.5）',
            },
          ]}
          help="可从列表选择或输入自定义值（如 0.5, 2, 4）"
        >
          <AutoComplete
            placeholder="选择或输入 CPU 核数"
            options={(config?.ui?.cpuOptions || ['2', '4', '8', '16']).map((cpu: string) => ({
              value: cpu,
              label: `${cpu} 核`,
            }))}
            filterOption={(inputValue, option) =>
              option?.value ? String(option.value).indexOf(inputValue) !== -1 : false
            }
          />
        </Form.Item>

        <Form.Item
          label="内存大小"
          name="memory"
          rules={[
            { required: true, message: '请输入内存大小' },
            {
              pattern: /^[0-9]+(\.[0-9]+)?(Mi|Gi)$/,
              message: '请输入有效的内存值（如 512Mi, 4Gi, 16Gi）',
            },
          ]}
          help="可从列表选择或输入自定义值（如 512Mi, 4Gi）"
        >
          <AutoComplete
            placeholder="选择或输入内存大小"
            options={(config?.ui?.memoryOptions || ['4Gi', '8Gi', '16Gi', '32Gi']).map((mem: string) => ({
              value: mem,
              label: mem,
            }))}
            filterOption={(inputValue, option) =>
              option?.value ? String(option.value).toUpperCase().indexOf(inputValue.toUpperCase()) !== -1 : false
            }
          />
        </Form.Item>

        {/* 配额预测 */}
        <Alert
          message={
            <div>
              <div>创建后配额使用：</div>
              <div style={{ marginTop: 8 }}>
                • Pod: {quotaCheck.newPodCount}/{currentQuota.podLimit}
                {quotaCheck.podExceeded && <span style={{ color: '#ff4d4f' }}> (超限)</span>}
              </div>
              <div>
                • GPU: {quotaCheck.newGPUCount}/{currentQuota.gpuLimit}
                {quotaCheck.gpuExceeded && <span style={{ color: '#ff4d4f' }}> (超限)</span>}
              </div>
            </div>
          }
          type={canCreate ? 'info' : 'error'}
          style={{ marginTop: 16 }}
        />

        <Alert
          message="所有 Pod 将在今晚 11:00 自动删除"
          type="warning"
          showIcon
          style={{ marginTop: 16 }}
        />
      </Form>
    </Modal>
  );
};

export default CreatePodModal;

