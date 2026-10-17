# Configuration Reference

Whether using Docker or Docker Compose (.env) deployment, configuration is done through environment variables.

## Environment Variables List

| Environment Variable                     | Description                                                     | Default                               | Required                                             |
| ---------------------------------------- | --------------------------------------------------------------- | ------------------------------------- | ---------------------------------------------------- |
| CFRAME_ADMIN_EMAIL                       | Initial admin user email                                        | `admin@memora.com`               | Yes                                                  |
| CFRAME_ADMIN_NAME                        | Initial admin username                                          | `Memora`                              | No                                                   |
| CFRAME_ADMIN_PASSWORD                    | Initial administrator password; set one of your own. There is no default, and the password ChronoFrame used to ship with is refused | None                                  | Required when creating the first administrator       |
| NUXT_PUBLIC_APP_TITLE                    | Application title                                               | `Memora`                         | No                                                   |
| NUXT_PUBLIC_APP_SLOGAN                   | Application slogan                                              | None                                  | No                                                   |
| NUXT_PUBLIC_APP_AUTHOR                   | Application author                                              | None                                  | No                                                   |
| NUXT_PUBLIC_APP_AVATAR_URL               | Application avatar URL (an image can also be uploaded from the dashboard and is then stored in the configured storage provider) | None                                  | No                                                   |
| NUXT_PUBLIC_COLOR_MODE_PREFERENCE        | Color mode preference, options: `light`, `dark`, `system`       | `system`                              | No                                                   |
| NUXT_PUBLIC_MAP_PROVIDER                 | Map provider, options: `mapbox`, `maplibre`                     | `maplibre`                            | No                                                   |
| NUXT_PUBLIC_MAPBOX_ACCESS_TOKEN          | Mapbox access token (URL restricted), for map services          | None                                  | Required when `NUXT_PUBLIC_MAP_PROVIDER` is `mapbox` |
| NUXT_NOMINATIM_BASE_URL                  | Nominatim base URL for reverse geocoding service                | `https://nominatim.openstreetmap.org` | No                                                   |
| NUXT_MAPBOX_ACCESS_TOKEN                 | Mapbox access token (no URL restriction), for location services | None                                  | No                                                   |
| NUXT_STORAGE_PROVIDER                    | Storage provider, supports `local`, `s3`, `openlist`            | `local`                               | Yes                                                  |
| NUXT_PROVIDER_LOCAL_PATH                 | Local storage path                                              | `/app/data/storage`                   | No                                                   |
| NUXT_PROVIDER_LOCAL_BASE_URL             | Local storage access URL                                        | `/storage`                            | No                                                   |
| NUXT_PROVIDER_S3_ENDPOINT                | S3 compatible storage service endpoint                          | None                                  | Required when `NUXT_STORAGE_PROVIDER` is `s3`        |
| NUXT_PROVIDER_S3_BUCKET                  | S3 bucket name                                                  | `memora`                         | Required when `NUXT_STORAGE_PROVIDER` is `s3`        |
| NUXT_PROVIDER_S3_REGION                  | S3 bucket region                                                | `auto`                                | Required when `NUXT_STORAGE_PROVIDER` is `s3`        |
| NUXT_PROVIDER_S3_ACCESS_KEY_ID           | S3 access key ID                                                | None                                  | Required when `NUXT_STORAGE_PROVIDER` is `s3`        |
| NUXT_PROVIDER_S3_SECRET_ACCESS_KEY       | S3 secret access key                                            | None                                  | Required when `NUXT_STORAGE_PROVIDER` is `s3`        |
| NUXT_PROVIDER_S3_PREFIX                  | S3 storage prefix                                               | `photos/`                             | No                                                   |
| NUXT_PROVIDER_S3_CDN_URL                 | S3 storage CDN URL                                              | None                                  | No                                                   |
| NUXT_PROVIDER_OPENLIST_BASE_URL          | OpenList server URL                                             | None                                  | Required when `NUXT_STORAGE_PROVIDER` is `openlist`  |
| NUXT_PROVIDER_OPENLIST_ROOT_PATH         | OpenList root path                                              | None                                  | Required when `NUXT_STORAGE_PROVIDER` is `openlist`  |
| NUXT_PROVIDER_OPENLIST_TOKEN             | OpenList API token                                              | None                                  | Recommended (for OpenList authentication)            |
| NUXT_PROVIDER_OPENLIST_ENDPOINT_UPLOAD   | OpenList upload endpoint                                        | `/api/fs/put`                         | No                                                   |
| NUXT_PROVIDER_OPENLIST_ENDPOINT_DOWNLOAD | OpenList download endpoint                                      | None                                  | No                                                   |
| NUXT_PROVIDER_OPENLIST_ENDPOINT_LIST     | OpenList list endpoint                                          | None                                  | No                                                   |
| NUXT_PROVIDER_OPENLIST_ENDPOINT_DELETE   | OpenList delete endpoint                                        | `/api/fs/remove`                      | No                                                   |
| NUXT_PROVIDER_OPENLIST_ENDPOINT_META     | OpenList metadata endpoint                                      | `/api/fs/get`                         | No                                                   |
| NUXT_PROVIDER_OPENLIST_PATH_FIELD        | OpenList path field name                                        | `path`                                | No                                                   |
| NUXT_PROVIDER_OPENLIST_CDN_URL           | OpenList CDN URL                                                | None                                  | No                                                   |
| NUXT_PUBLIC_OAUTH_GITHUB_ENABLED         | Enable GitHub OAuth login                                       | `false`                               | No                                                   |
| NUXT_OAUTH_GITHUB_CLIENT_ID              | GitHub OAuth app Client ID                                      | None                                  | No (optional, for GitHub login)                      |
| NUXT_OAUTH_GITHUB_CLIENT_SECRET          | GitHub OAuth app Client Secret                                  | None                                  | No (optional, for GitHub login)                      |
| NUXT_SESSION_PASSWORD                    | Password for encrypting sessions, 32-character random string    | None                                  | Yes                                                  |
| CFRAME_WORKERS                           | How many tasks the queue runs at once (1-8) when the process starts. The queue page in the dashboard changes the same number while the server is running. | `5` | No |
| CFRAME_OG_FONT                           | Font file a link preview card is drawn in. Empty draws it in the Go font, and in a font the machine keeps for Chinese, Japanese and Korean | None | No |
| NUXT_PUBLIC_GTAG_ID                      | Google Analytics Tracking ID                                    | None                                  | No                                                   |
| NUXT_PUBLIC_ANALYTICS_MATOMO_ENABLED     | Enable Matomo analytics tracking                                | `false`                               | No                                                   |
| NUXT_PUBLIC_ANALYTICS_MATOMO_URL         | Matomo instance URL (e.g., https://matomo.example.com)          | None                                  | No (required when Matomo is enabled)                 |
| NUXT_PUBLIC_ANALYTICS_MATOMO_SITE_ID     | Matomo site ID                                                  | None                                  | No (required when Matomo is enabled)                 |
| NUXT_UPLOAD_MIME_WHITELIST_ENABLED       | Enable MIME type whitelist validation for uploads               | `true`                                | No                                                   |
| NUXT_UPLOAD_MIME_WHITELIST               | Allowed MIME types for uploads (comma-separated)                | See below                             | No                                                   |
| ALLOW_INSECURE_COOKIE                    | Allow insecure cookies (only for development environment)       | `false`                               | No                                                   |

## Link Preview Cards

A link to this site pasted into a chat client or a timeline is shown as a card:
a 1200×630 JPEG the server draws when the client asks for it, rather than
whichever photo happens to be first. The card of the site carries the pictures
it holds, its name, its slogan and who it belongs to; a site with no pictures
yet carries the picture it goes by instead. The card of a photo carries the
photo, its title, its description and what the file says about the camera. The
size is declared in the page as well (`og:image:width` and `og:image:height`),
so a client can lay the preview out before it has the picture.

| Address             | What it draws                                                |
| ------------------- | ------------------------------------------------------------ |
| `/_og/site.jpg`     | The site: its pictures, name, slogan, author and picture count |
| `/_og/photo/<id>.jpg` | That photo, when an anonymous visitor may see it             |

The pictures a card offers are the ones an anonymous visitor may see, and the
card says how many there are in the language the page that named it was served
in. Cards are kept for ten minutes (`Cache-Control: public, max-age=600`) and
drawn again after that, so a change to the title or the avatar reaches a preview
without a restart. A photo in a hidden or password-protected album has no card:
the address answers `404`, the same way the photo is kept out of the page.

Cards are set in the Go font, which ships inside the Go image library and has
Latin, Greek and Cyrillic letters but no Chinese, Japanese or Korean ones. For
text it cannot write, a card is set in a font the machine already keeps
(Microsoft YaHei, PingFang, Noto Sans CJK and friends). To choose the type
yourself — for a language neither of those has, or for a font that matches your
design — name a `.ttf`, `.otf` or `.ttc` file:

```bash
CFRAME_OG_FONT=/app/data/fonts/NotoSansSC-Regular.otf
```

A named font sets the whole card, including the heavy lines. If the file cannot
be read, the server says so in the system log and draws the card in the Go font
instead: a link still arrives with a preview.

## Upload File Type Whitelist

The default value of `NUXT_UPLOAD_MIME_WHITELIST` includes the following MIME types:

- `image/jpeg` - JPEG images
- `image/png` - PNG images
- `image/webp` - WebP images
- `image/gif` - GIF images
- `image/bmp` - BMP images
- `image/tiff` - TIFF images
- `image/heic` - HEIC images (Apple)
- `image/heif` - HEIF images
- `video/quicktime` - QuickTime videos (MOV)
- `video/mp4` - MP4 videos

To customize the whitelist, use a comma-separated string of MIME types, for example:

```
NUXT_UPLOAD_MIME_WHITELIST=image/jpeg,image/png,video/mp4
```

To disable whitelist validation (allow any file type), set:

```
NUXT_UPLOAD_MIME_WHITELIST_ENABLED=false
```
