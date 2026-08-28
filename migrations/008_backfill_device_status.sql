UPDATE devices
   SET status = 'active'
 WHERE device_token_hash IS NOT NULL
   AND enrolled_at IS NOT NULL