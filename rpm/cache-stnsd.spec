Summary: Caching response from stns server.
Name:             cache-stnsd
Version:          0.3.0
Release:          1
License:          GPLv3
URL:              https://github.com/STNS/cache-stnsd
Group:            System Environment/Base
Packager:         pyama86 <www.kazu.com@gmail.com>
Source:           %{name}-%{version}.tar.gz
BuildRequires:    make
BuildRoot:        %{_tmppath}/%{name}-%{version}-%{release}-root-%(%{__id_u} -n)
BuildArch:        i386, x86_64

%ifarch x86_64
%global gohostarch  amd64
%endif
%ifarch %{ix86}
%global gohostarch  386
%endif
%ifarch %{arm}
%global gohostarch  arm
%endif
%ifarch aarch64
%global gohostarch  arm64
%endif
%define debug_package %{nil}

%description
This process provided caching response from stns.

%prep
%setup -q -n %{name}-%{version}

%build
export GOOS=linux
export GOARCH=%{gohostarch}
make

%install
%{__rm} -rf %{buildroot}
mkdir -p %{buildroot}/usr/sbin
make PREFIX=%{buildroot}/usr/ install

%if 0%{?rhel} < 7
mkdir -p %{buildroot}%{_sysconfdir}/init.d
install -m 755 package/cache-stnsd.initd  %{buildroot}%{_sysconfdir}/init.d/cache-stnsd
%else
mkdir -p %{buildroot}%{_sysconfdir}/systemd/system/
install -m 755 package/cache-stnsd.systemd %{buildroot}%{_sysconfdir}/systemd/system/cache-stnsd.service
%endif

mkdir -p %{buildroot}%{_sysconfdir}/logrotate.d
install -m 644 package/cache-stnsd.logrotate %{buildroot}%{_sysconfdir}/logrotate.d/cache-stnsd

%clean
%{__rm} -rf %{buildroot}

%post
if [ `which service` ]; then
  ! test -e  /etc/stns/client/stns.conf || service cache-stnsd start
elif [ `which systemctl` ]; then
  ! test -e /etc/stns/client/stns.conf || systemctl start cache-stnsd
fi

%preun

%postun

%files
%defattr(-, root, root)
/usr/sbin/cache-stnsd
/etc/logrotate.d/cache-stnsd

%if 0%{?rhel} < 7
/etc/init.d/cache-stnsd
%else
/etc/systemd/system/cache-stnsd.service
%endif

%changelog
* Thu Aug 13 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.0-1
- Add prefetch method
* Wed Aug 12 2020 pyama86 <www.kazu.com@gmail.com> - 0.2.0-1
- Change cache method to ttlcache
* Tue Jul 14 2020 pyama86 <www.kazu.com@gmail.com> - 0.1.0-1
- Initial packaging
