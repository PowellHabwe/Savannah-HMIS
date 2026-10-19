# Deployment guide: DigitalOcean droplet (Ubuntu 22.04)

How it works: a pull request runs the tests. Merging to `main` runs the tests again, then GitHub connects to the droplet over SSH, the droplet fetches that exact commit from the public repo, builds the Docker image itself, and restarts the stack with Docker Compose. There is no image registry.

## 0. Test the production stack on your own machine first (PowerShell)

Run from the project root with Docker Desktop running. This uses the real production compose file with throw-away credentials:

```powershell
$d = "$env:TEMP\hmis-test"; New-Item -ItemType Directory -Force $d | Out-Null
@"
DB_USER=clinic
DB_PASSWORD=localtest
DB_NAME=clinic
SITE_ADDRESS=:80
"@ | Set-Content "$d\.env"
$dc = "docker compose --env-file $d\.env -f deploy\docker-compose.prod.yml"
Invoke-Expression "$dc up -d --build"
Invoke-Expression "$dc ps"
curl http://localhost/health
```

Expected: `postgres`, `api` and `caddy` running, `migrate` exited (0), and `/health` returns `{"data":{"status":"ok"}}`. Then:

```powershell
Invoke-Expression "$dc run --rm --entrypoint ./seed api"
curl http://localhost/api/v1/doctors
```

Clean up. This only deletes the test stack's own database, not your dev one:

```powershell
Invoke-Expression "$dc down -v"
```

If port 80 is busy on your PC (often IIS), stop that program or skip this step.

## 1. Push the code to GitHub

The server fetches the code from GitHub, so it must be pushed first (the repo is public):

```powershell
git push -u origin main
```

## 2. Create the droplet (DigitalOcean website)

- Image: **Ubuntu 22.04 (LTS) x64**
- Size: Basic, **1 GB RAM / 1 vCPU** (swap is added below so the build fits)
- Authentication: **SSH key**
- Note the droplet's **public IPv4 address**.

## 3. Create a deploy key for GitHub Actions (your PowerShell)

```powershell
ssh-keygen -t ed25519 -C "github-actions-deploy" -f "$env:USERPROFILE\.ssh\savannah_deploy"
```

Press Enter twice for an empty passphrase. This creates a private key (`savannah_deploy`) and a public key (`savannah_deploy.pub`).

## 4. Prepare the server

```powershell
ssh root@YOUR_DROPLET_IP
```

Install Docker, git and the firewall, and add swap (the Go build needs memory):

```bash
apt-get update && apt-get -y upgrade
apt-get -y install ca-certificates curl ufw git
curl -fsSL https://get.docker.com | sh
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
fallocate -l 1G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

Create the deploy user and the app folder:

```bash
adduser --disabled-password --gecos "" deploy
usermod -aG docker deploy
mkdir -p /home/deploy/.ssh /opt/savannah-hmis
chown -R deploy:deploy /opt/savannah-hmis
```

Authorise the deploy key. On your PC, print the PUBLIC key and copy it:

```powershell
Get-Content "$env:USERPROFILE\.ssh\savannah_deploy.pub"
```

On the server, paste it between the quotes:

```bash
echo "PASTE_PUBLIC_KEY_HERE" >> /home/deploy/.ssh/authorized_keys
chown -R deploy:deploy /home/deploy/.ssh
chmod 700 /home/deploy/.ssh
chmod 600 /home/deploy/.ssh/authorized_keys
```

Generate a database password, then create the server's `.env`:

```bash
openssl rand -hex 24
```

```bash
cat > /opt/savannah-hmis/.env <<'ENVEOF'
DB_USER=clinic
DB_PASSWORD=PASTE_THE_GENERATED_PASSWORD
DB_NAME=clinic
SITE_ADDRESS=:80
ENVEOF
chown deploy:deploy /opt/savannah-hmis/.env
chmod 600 /opt/savannah-hmis/.env
```

This file lives only on the server and is git-ignored, so deployments never overwrite it.

Check the deploy user can log in and use Docker. From your PC:

```powershell
ssh -i "$env:USERPROFILE\.ssh\savannah_deploy" deploy@YOUR_DROPLET_IP "docker ps"
```

An empty table (no error) means it works.

## 5. GitHub repository secrets

Repo → Settings → Secrets and variables → Actions → New repository secret:

| Name | Value |
|---|---|
| `DROPLET_HOST` | the droplet IP |
| `DROPLET_USER` | `deploy` |
| `DROPLET_SSH_KEY` | the PRIVATE key: `Get-Content "$env:USERPROFILE\.ssh\savannah_deploy" -Raw \| Set-Clipboard` then paste |
| `APP_URL` | `http://YOUR_DROPLET_IP` |

## 6. First deployment

1. Trigger the workflow: push any commit to `main`, or Actions → Deploy → re-run. The first build on a small droplet takes a few minutes.
2. When the run is green, seed the demo doctors and patients once:

```powershell
ssh deploy@YOUR_DROPLET_IP "cd /opt/savannah-hmis && docker compose --env-file .env -f deploy/docker-compose.prod.yml run --rm --entrypoint ./seed api"
```

3. Check it:

```powershell
curl http://YOUR_DROPLET_IP/health
curl http://YOUR_DROPLET_IP/api/v1/doctors
```

## 7. Protect `main` (recommended)

Repo → Settings → Branches → Add rule for `main`: require a pull request and require the `test` status check to pass. Then nothing reaches `main` (and so production) without green tests.

## Optional: HTTPS without a domain

Use the free `sslip.io` service, which maps `203-0-113-10.sslip.io` to `203.0.113.10`. On the server, edit `/opt/savannah-hmis/.env` and set:

```
SITE_ADDRESS=YOUR-IP-WITH-DASHES.sslip.io
```

Update the `APP_URL` secret to `https://YOUR-IP-WITH-DASHES.sslip.io`, then redeploy. Caddy obtains a certificate automatically. This depends on Let's Encrypt agreeing to issue for the shared sslip.io domain; if it fails, set it back to `:80`.

## Operating notes

- Short command for use on the server (add to `~/.bashrc` of `deploy`):
  `alias hmis='docker compose --env-file /opt/savannah-hmis/.env -f /opt/savannah-hmis/deploy/docker-compose.prod.yml'`
  then `hmis logs -f api`, `hmis ps`.
- Roll back: on the server, `cd /opt/savannah-hmis && git fetch --depth 1 origin <old-sha> && git checkout -f -B deploy FETCH_HEAD`, then `hmis up -d --build`.
- Disk: old build cache accumulates. Clean it now and then with `docker builder prune -f`.
- Database backups are not automated here (a growth item).
