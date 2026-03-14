import { DownOutlined, SettingOutlined, UserOutlined } from '@ant-design/icons';
import { Avatar, Button, Dropdown, Space, Tag } from 'antd';
import React from 'react';
import './UserMenu.css';

interface UserMenuProps {
  username?: string;
  isAdmin?: boolean;
  poolType?: 'shared' | 'exclusive';
  onPrimaryAction: () => void;
}

const UserMenu: React.FC<UserMenuProps> = ({ username, isAdmin, poolType, onPrimaryAction }) => {
  const items = [
    {
      key: 'primary',
      icon: isAdmin ? <SettingOutlined /> : <UserOutlined />,
      label: isAdmin ? '管理员页' : '个人详情',
      onClick: onPrimaryAction,
    },
  ];

  return (
    <Dropdown menu={{ items }} trigger={['click']} placement="bottomRight">
      <Button className="user-menu-trigger glass-button">
        <Space size={10}>
          <Avatar size={30} icon={<UserOutlined />} className="user-menu-avatar" />
          <span className="user-menu-name">{username || 'unknown'}</span>
          <Tag className={`user-menu-pool user-menu-pool-${poolType || 'shared'}`}>
            {poolType || 'shared'}
          </Tag>
          <DownOutlined className="user-menu-caret" />
        </Space>
      </Button>
    </Dropdown>
  );
};

export default UserMenu;
