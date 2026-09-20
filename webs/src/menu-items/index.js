import dashboard from './dashboard';
import { subscription, script, accesskey, system } from './subscription';
import { IconTicket } from '@tabler/icons-react';

// ==============================|| MENU ITEMS ||============================== //

const distribution = {
  id: 'distribution-group',
  title: 'Distribution',
  titleKey: 'distribution.title',
  type: 'group',
  children: [
    { id: 'distribution', title: 'Distribution', titleKey: 'distribution.title', type: 'item', url: '/distribution', icon: IconTicket }
  ]
};
const adminItem = (item) => ({
  ...item,
  ...(item.url?.startsWith('/') ? { url: `/admin${item.url}` } : {}),
  ...(item.children ? { children: item.children.map(adminItem) } : {})
});
const menuItems = { items: [dashboard, distribution, subscription, script, accesskey, system].map(adminItem) };

export default menuItems;
