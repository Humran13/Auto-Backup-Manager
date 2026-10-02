# Google Drive

| | |
|---|---|
| **Provider ID** | `google-drive` |
| **Maturity** | Stable (OAuth credentials/account required) |
| **Backend** | rclone Drive, managed internally by ABM |

In the GUI choose **Storage → Google Drive → Connect**. Enter a destination
name plus a Google Desktop OAuth Client ID and Client Secret from your own
Google Cloud project. ABM starts authorization, opens/shows the provider page,
stores the resulting token in its protected rclone config, and never returns
or displays the token.

Google's policies make a shared default client unsuitable for unattended
production use, so bring-your-own OAuth credentials are required. Public CI
tests the UI/orchestration boundary without faking a successful Google login.
On headless VPS installations, rclone's localhost OAuth callback can require a
secure tunnel/browser arrangement; ABM never reports Connected until rclone
returns a real token.
