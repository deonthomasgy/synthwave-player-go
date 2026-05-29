Name:           synthwave-player
Version:        %{tag_version}
Release:        1%{?dist}
Summary:        Launcher for the Synthwave Player web interface

License:        GPL-3.0-or-later
URL:            https://github.com/Elive/synthwave-player
Source0:        synthwave-player-%{version}.tar.gz

BuildArch:      noarch

Requires:       synthwave-player-server = %{version}-%{release}
Recommends:     (google-chrome-stable or chromium or firefox), zenity, nc

%description
This package provides a desktop launcher to easily open the Synthwave Player
web interface.

%package -n synthwave-player-server
Summary:        A music player server with a synthwave-themed web interface
BuildRequires:  golang
Requires:       file
Requires:       eyed3

%description -n synthwave-player-server
A music player server with a synthwave-themed web interface. It scans your
music library, and serves a web interface to browse and play your music.
It supports playlists, cover art, and UPnP for remote access.

%prep
%setup -q -n tree

%build
cd ..
go build -ldflags="-s -w" -o tree/synthwave-player-server ./cmd/synthwave-player-server
cd tree

%install
mkdir -p %{buildroot}%{_libdir}/synthwave-player
install -m 755 synthwave-player-server %{buildroot}%{_libdir}/synthwave-player/synthwave-player-server

mkdir -p %{buildroot}%{_datadir}/synthwave-player
cp -r usr/share/synthwave-player/public %{buildroot}%{_datadir}/synthwave-player/
cp -r usr/share/synthwave-player/templates %{buildroot}%{_datadir}/synthwave-player/

mkdir -p %{buildroot}%{_bindir}
install -m 755 usr/bin/synthwave-player-server %{buildroot}%{_bindir}/synthwave-player-server
install -m 755 usr/bin/synthwave-player %{buildroot}%{_bindir}/synthwave-player

mkdir -p %{buildroot}%{_datadir}/applications
cp usr/share/applications/synthwave-player.desktop %{buildroot}%{_datadir}/applications/
cp usr/share/applications/synthwave-player-server.desktop %{buildroot}%{_datadir}/applications/

mkdir -p %{buildroot}%{_datadir}/icons/hicolor/scalable/apps
install -m 644 usr/share/synthwave-player/pixmaps/synthwave-player.png %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/
install -m 644 usr/share/synthwave-player/pixmaps/synthwave-player-rounded.png %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/
install -m 644 usr/share/synthwave-player/pixmaps/synthwave-player-server.png %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/

%files
%{_bindir}/synthwave-player
%{_datadir}/applications/synthwave-player.desktop
%{_datadir}/icons/hicolor/scalable/apps/synthwave-player.png
%{_datadir}/icons/hicolor/scalable/apps/synthwave-player-rounded.png

%files -n synthwave-player-server
%{_bindir}/synthwave-player-server
%{_libdir}/synthwave-player/synthwave-player-server
%{_datadir}/synthwave-player
%{_datadir}/applications/synthwave-player-server.desktop
%{_datadir}/icons/hicolor/scalable/apps/synthwave-player-server.png

%changelog
* Fri Aug 29 2025 Thanatermesis <thanatermesis@elivecd.org> - %{version}-1
- Initial RPM release for version %{version}
