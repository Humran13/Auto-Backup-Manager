# Microsoft OneDrive

| | |
|---|---|
| **Provider ID** | `onedrive` |
| **Maturity** | Stable |
| **Backend** | rclone OneDrive, managed internally by ABM |

Choose **Storage → Microsoft OneDrive → Connect**. Sign in on Microsoft's
authorization page; ABM captures and stores the token without displaying it.
Business/SharePoint tenants with restrictive policies may require an
administrator-approved Azure application. Public CI validates the graphical
flow boundary but does not fake a live Microsoft account login.
