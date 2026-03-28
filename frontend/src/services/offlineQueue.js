import localforage from 'localforage';

const REFERRAL_QUEUE_KEY = 'medconnect_offline_referrals_v1';
const OFFLINE_AES_KEY_KEY = 'medconnect_offline_aes_key_v1';

localforage.config({
  name: 'medconnect',
  storeName: 'offline_queue',
});

function bytesToBase64(bytes) {
  let binary = '';
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

function base64ToBytes(base64) {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function getOrCreateRawKey() {
  let keyB64 = localStorage.getItem(OFFLINE_AES_KEY_KEY);
  if (!keyB64) {
    const keyBytes = new Uint8Array(32);
    crypto.getRandomValues(keyBytes);
    keyB64 = bytesToBase64(keyBytes);
    localStorage.setItem(OFFLINE_AES_KEY_KEY, keyB64);
  }
  return base64ToBytes(keyB64);
}

async function getCryptoKey() {
  const rawKey = getOrCreateRawKey();
  return crypto.subtle.importKey('raw', rawKey, 'AES-GCM', false, ['encrypt', 'decrypt']);
}

async function encryptPayload(payload) {
  const key = await getCryptoKey();
  const iv = new Uint8Array(12);
  crypto.getRandomValues(iv);

  const plaintext = new TextEncoder().encode(JSON.stringify(payload));
  const encrypted = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, plaintext);

  return {
    iv: bytesToBase64(iv),
    ciphertext: bytesToBase64(new Uint8Array(encrypted)),
  };
}

async function decryptPayload(encryptedPayload) {
  const key = await getCryptoKey();
  const iv = base64ToBytes(encryptedPayload.iv);
  const ciphertext = base64ToBytes(encryptedPayload.ciphertext);

  const decrypted = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ciphertext);
  return JSON.parse(new TextDecoder().decode(decrypted));
}

async function getQueue() {
  return (await localforage.getItem(REFERRAL_QUEUE_KEY)) || [];
}

async function setQueue(queue) {
  await localforage.setItem(REFERRAL_QUEUE_KEY, queue);
}

export async function enqueueReferralPayload(payload) {
  const encrypted = await encryptPayload(payload);
  const queue = await getQueue();

  queue.push({
    id: crypto.randomUUID(),
    encrypted,
    createdAt: new Date().toISOString(),
  });

  await setQueue(queue);
  return queue.length;
}

export async function getQueuedReferralCount() {
  const queue = await getQueue();
  return queue.length;
}

export async function flushQueuedReferrals(sendFn) {
  const queue = await getQueue();
  if (queue.length === 0) {
    return { sent: 0, failed: 0 };
  }

  let sent = 0;
  let failed = 0;
  const remaining = [];

  for (const item of queue) {
    try {
      const payload = await decryptPayload(item.encrypted);
      await sendFn(payload);
      sent += 1;
    } catch (error) {
      failed += 1;
      remaining.push(item);
      console.error('[OFFLINE SYNC] Failed to sync queued referral', error);
    }
  }

  await setQueue(remaining);
  return { sent, failed };
}
