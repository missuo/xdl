# Changelog

## 2.3.0

- **All of XDL is its own code now**: the downloads, the library and the
  FFmpeg / ExifTool integration are rewritten, and nothing of
  Twitter-X-Media-Batch-Downloader is left. File names and folders stay as
  they were, so downloads made before are still found and skipped.
- **No downloaded programs**: the FFmpeg that older versions installed came
  from builds afkarxyz uploaded; XDL deletes it. FFmpeg and ExifTool now
  come from your package manager (`brew install ffmpeg exiftool`, `winget
  install Gyan.FFmpeg OliverBetz.ExifTool`, `apt install ffmpeg
  libimage-exiftool-perl`); Settings → Dependencies shows the command.
- **One data folder**: the library moves from `~/.twitterxmediabatchdownloader`
  to XDL's folder (`~/Library/Application Support/XDL` on macOS), and the
  auth token to the keychain entry "XDL", by themselves.
- XDL 的全部代码现在都是自己写的：下载、资料库、FFmpeg / ExifTool 集成都已重写，不再包含 Twitter-X-Media-Batch-Downloader 的任何代码。文件名和目录结构保持不变，以前下载过的文件照样能识别、跳过。
- 不再下载任何程序：旧版本安装的 FFmpeg 来自 afkarxyz 上传的构建，XDL 会把它删掉。FFmpeg 和 ExifTool 改用系统包管理器安装，「设置 → 依赖」里会显示安装命令。
- 数据统一放到 XDL 自己的目录：资料库从 `~/.twitterxmediabatchdownloader` 自动搬到 XDL 的目录（macOS 上是 `~/Library/Application Support/XDL`），auth token 也自动迁移到钥匙串里名为「XDL」的条目。

## 2.2.0

- **Automatic updates**: XDL checks for a new version in the background
  and offers to install it, with its release notes, as Sparkle does on the
  Mac (macOS, Windows, and Linux installs from `install.sh`). Settings →
  General has "Check for Updates…" and the automatic checks; the menu bar
  icon has "Check for Updates…" too. Updates are signed: XDL installs only
  what its release key signed.
- 自动更新：XDL 会在后台检查新版本，并像 Mac 上的 Sparkle 一样显示更新说明、提示安装（macOS、Windows，以及用 `install.sh` 安装的 Linux）。设置 → 通用里可以手动检查、开关自动检查，菜单栏图标里也有「检查更新…」。更新包有签名，XDL 只安装用发布密钥签过名的版本。

## 2.1.0

- **Profile page**: open an account from the Library to see its profile and
  its media as a grid of thumbnails, filter by type, and look through it in
  a viewer that plays videos; download one item, a selection or all of it.
- 个人主页：在资料库里打开账号，查看资料和媒体缩略图网格，按类型筛选，在查看器里浏览（视频可直接播放），单张、多选或全部下载。

## 2.0.1

- **Security fix**: XDL no longer downloads or runs xtractor, which uploaded
  users' auth tokens to its author. XDL reads X with its own open-source
  code, and deletes the xtractor older versions left. Revoke any token used
  with 2.0.0.
- 安全修复：XDL 不再下载或运行 xtractor（它会把用户的 auth token 上传给作者），改用自己的开源代码读取 X，并删除旧版本留下的 xtractor。用过 2.0.0 的 token 请作废。
