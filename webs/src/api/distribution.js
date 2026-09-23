import request from './request';

const root = '/v1/distribution';
export const distributionAPI = {
  list: (resource, params) =>
    ['credentials', 'cards'].includes(resource) && params?.keyword?.trim()
      ? request.post(`${root}/${resource}/search`, params)
      : request.get(`${root}/${resource}`, { params }),
  getCredential: (id) => request.get(`${root}/credentials/${id}`),
  listAccessGrants: (id) => request.get(`${root}/credentials/${id}/access-grants`),
  createAccessGrant: (id, data) => request.post(`${root}/credentials/${id}/access-grants`, data),
  revokeAccessGrant: (id, grantId) => request.post(`${root}/credentials/${id}/access-grants/${grantId}/revoke`),
  getCard: (id) => request.get(`${root}/cards/${id}`),
  issue: (data) => request.post(`${root}/credentials`, data),
  updateCredential: (id, data) => request.patch(`${root}/credentials/${id}`, data),
  deleteCredentials: (ids) => request.post(`${root}/credentials/batch-delete`, { ids }),
  createCards: (data) => request.post(`${root}/cards`, data),
  currentFreeCard: () => request.post(`${root}/cards/free-cycle`),
  reissueFreeCard: () => request.post(`${root}/cards/free-cycle/reissue`),
  updateCard: (id, data) => request.patch(`${root}/cards/${id}`, data),
  deleteCards: (ids) => request.post(`${root}/cards/batch-delete`, { ids }),
  review: (id, data) => request.post(`${root}/region-requests/${id}/review`, data),
  pendingRegions: (signal) => request.get(`${root}/region-requests`, { params: { status: 'pending', page: 1, size: 1 }, signal }),
  settings: () => request.get(`${root}/settings`),
  saveSettings: (data) => request.put(`${root}/settings`, data)
};

// Public requests never attach the administrator's JWT and cannot log them out.
export async function distributionPublic(resource, data) {
  const response = await fetch(`/api/public/distribution/${resource}`, {
    method: data === undefined ? 'GET' : 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'omit',
    cache: 'no-store',
    signal: AbortSignal.timeout(15000),
    ...(data === undefined ? {} : { body: JSON.stringify(data) })
  });
  const result = await response.json();
  if (!response.ok || result.code !== 200) {
    const error = new Error(result.msg || 'server_error');
    error.i18nKey = result.i18nKey;
    throw error;
  }
  return result.data;
}
