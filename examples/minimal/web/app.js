const button = document.querySelector('#notifications');
async function request(path, options) {
  const response = await fetch(path, options);
  if (!response.ok) throw new Error(`Notification setup failed (${response.status})`);
  return response.status === 204 ? null : response.json();
}
const client = PWAKit.createPushClient({
  getPublicKey: async () => (await request('/api/push/key')).public_key,
  save: sub => request('/api/push/subscriptions', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(sub.toJSON())}),
  remove: sub => request('/api/push/subscriptions', {method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({endpoint:sub.endpoint})}),
  onState: state => {
    button.disabled = ['checking','enabling','disabling','install','unsupported','blocked','signedOut'].includes(state.status);
    button.textContent = state.status === 'on' ? '✓ Notifications on — disable' : 'Enable notifications';
    document.querySelector('#status').textContent = state.message;
  }
});
button.addEventListener('click', () => client.state.status === 'on' ? client.disable() : client.enable());
client.refresh();
