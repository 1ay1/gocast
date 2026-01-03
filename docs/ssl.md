# SSL/HTTPS Setup

GoCast supports HTTPS to secure your streams. You can use automatic certificates from Let's Encrypt (AutoSSL) or provide your own certificates.

## Quick Start with AutoSSL

The easiest way to enable SSL is with AutoSSL:

1. **Set your hostname** in the Admin Panel → Settings → Server
2. **Enable AutoSSL** in Settings → SSL
3. **Restart GoCast**

That's it! GoCast will automatically obtain and renew certificates.

## AutoSSL (Let's Encrypt)

AutoSSL uses Let's Encrypt to automatically obtain and renew free SSL certificates.

### Requirements

- A valid public domain name (not localhost)
- DNS pointing to your server's IP address
- Port 80 accessible from the internet (for Let's Encrypt verification)
- Port 8443 accessible (for HTTPS traffic)

### Configuration via Admin Panel

1. Go to **Settings → SSL**
2. Enter your **Domain Name** (e.g., `radio.example.com`)
3. Enter your **Email** (optional, for expiry notifications)
4. Click **Enable AutoSSL**
5. Restart GoCast

### Configuration via Config File

```json
{
  "server": {
    "hostname": "radio.example.com"
  },
  "ssl": {
    "enabled": true,
    "port": 8443,
    "auto_ssl": true,
    "auto_ssl_email": "admin@example.com"
  }
}
```

Then restart GoCast:
```bash
pkill gocast && ./gocast
```

### How AutoSSL Works

1. GoCast starts an HTTP server on port 80 for ACME challenges
2. Let's Encrypt verifies domain ownership
3. Certificate is obtained and cached
4. HTTPS server starts on port 8443
5. HTTP traffic on port 80 redirects to HTTPS
6. Certificates auto-renew before expiry

### Certificate Storage

Certificates are cached in:
```
~/.gocast/certs/
```

Or if using a custom data directory:
```
/your/data/dir/certs/
```

## Manual SSL Certificates

If you have your own SSL certificates (from a commercial CA or self-signed), you can use them instead of AutoSSL.

### Configuration via Admin Panel

1. Go to **Settings → SSL**
2. Expand **Manual SSL Configuration**
3. Enter:
   - **SSL Port**: 8443 (or your preferred port)
   - **Certificate Path**: `/path/to/fullchain.pem`
   - **Private Key Path**: `/path/to/privkey.pem`
4. Click **Save Manual SSL Settings**
5. Restart GoCast

### Configuration via Config File

```json
{
  "ssl": {
    "enabled": true,
    "port": 8443,
    "auto_ssl": false,
    "cert_path": "/etc/ssl/certs/radio.example.com.crt",
    "key_path": "/etc/ssl/private/radio.example.com.key"
  }
}
```

### Using Let's Encrypt Certbot

If you prefer to manage certificates with Certbot:

```bash
# Obtain certificate
sudo certbot certonly --standalone -d radio.example.com

# Configure GoCast
{
  "ssl": {
    "enabled": true,
    "port": 8443,
    "auto_ssl": false,
    "cert_path": "/etc/letsencrypt/live/radio.example.com/fullchain.pem",
    "key_path": "/etc/letsencrypt/live/radio.example.com/privkey.pem"
  }
}
```

Set up auto-renewal in a cron job:
```bash
0 0 * * * certbot renew --quiet && pkill -HUP gocast
```

### Self-Signed Certificates (Testing Only)

Generate a self-signed certificate for testing:

```bash
openssl req -x509 -nodes -days 365 \
  -newkey rsa:2048 \
  -keyout server.key \
  -out server.crt \
  -subj "/CN=localhost"
```

**Warning:** Self-signed certificates will show browser warnings and are not suitable for production.

## Ports

### Default Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 8000 | HTTP | Main server (streams + admin) |
| 8443 | HTTPS | SSL/TLS traffic |
| 80 | HTTP | AutoSSL verification + redirect |

### Using Standard Ports (443)

To use port 443 instead of 8443:

```json
{
  "ssl": {
    "port": 443
  }
}
```

**Note:** Ports below 1024 require root privileges or capabilities:

```bash
# Option 1: Run as root (not recommended)
sudo ./gocast

# Option 2: Use setcap (Linux)
sudo setcap 'cap_net_bind_service=+ep' ./gocast

# Option 3: Use a reverse proxy (recommended for production)
```

## Stream URLs

Once SSL is enabled, streams are available at:

```
https://radio.example.com:8443/live
```

Both HTTP and HTTPS URLs work simultaneously:
```
http://radio.example.com:8000/live   (HTTP)
https://radio.example.com:8443/live  (HTTPS)
```

## Reverse Proxy Setup

For production deployments, consider using a reverse proxy like nginx.

### nginx Configuration

```nginx
server {
    listen 80;
    server_name radio.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name radio.example.com;

    ssl_certificate /etc/letsencrypt/live/radio.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/radio.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # For streaming
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 24h;
    }
}
```

In this setup, GoCast listens on HTTP only, and nginx handles SSL termination.

## Troubleshooting

### Certificate Not Obtained

```
failed to obtain certificate
```

- Verify DNS is pointing to your server: `dig radio.example.com`
- Check port 80 is accessible: `curl http://your-ip:80/`
- Ensure no other service is using port 80
- Check firewall rules

### Connection Refused on HTTPS

- Verify GoCast restarted after enabling SSL
- Check port 8443 is open: `netstat -tlnp | grep 8443`
- Check firewall allows port 8443

### Certificate Expired

AutoSSL certificates renew automatically. If expired:

1. Delete the cache: `rm -rf ~/.gocast/certs/`
2. Restart GoCast

For manual certificates, renew and restart:
```bash
certbot renew
pkill -HUP gocast
```

### Browser Shows "Not Secure"

- Self-signed certificates always show warnings
- Ensure you're using the correct domain (not IP address)
- Check certificate includes correct domain name

### Mixed Content Warnings

If embedding HTTPS streams in an HTTP page (or vice versa), browsers may block it. Use consistent protocols.

## Security Best Practices

1. **Always use HTTPS in production** - Protect listener privacy
2. **Use AutoSSL or trusted CA** - Avoid self-signed certificates
3. **Keep certificates updated** - AutoSSL handles this automatically
4. **Use strong TLS settings** - GoCast defaults to TLS 1.2+
5. **Protect private keys** - Restrict file permissions to 600
6. **Use a reverse proxy** - For additional security layers