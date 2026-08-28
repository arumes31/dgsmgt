# Security policy

## Supported versions

Only the latest release and the default branch receive security fixes.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting feature for this repository. Do
not open a public issue or include credentials, access tokens, customer data,
or exploit details in logs or pull requests.

Include the affected version, reproduction conditions, impact, and any known
mitigations. You should receive an acknowledgement within three business days.

## Deployment responsibility

Operators must replace installation credentials, keep the management endpoint
on a trusted network, use digest-pinned allowed images, and restrict host bind
sources to exact dedicated directories. Docker access is root-equivalent even
though the public web process is isolated from the host socket.
