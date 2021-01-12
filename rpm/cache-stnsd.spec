Summary: Caching response from stns server.
Name:             cache-stnsd
Version:          0.3.9
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
install -m 644 package/cache-stnsd.systemd %{buildroot}%{_sysconfdir}/systemd/system/cache-stnsd.service
%endif

mkdir -p %{buildroot}%{_sysconfdir}/logrotate.d
install -m 644 package/cache-stnsd.logrotate %{buildroot}%{_sysconfdir}/logrotate.d/cache-stnsd

%clean
%{__rm} -rf %{buildroot}

%post
if [ `which systemctl` ]; then
  ! test -e /etc/stns/client/stns.conf || (systemctl daemon-reload && systemctl restart cache-stnsd)
elif [ `which service` ]; then
  ! test -e  /etc/stns/client/stns.conf || service cache-stnsd restart
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
* Mon Jan 12 2021 pyama86 <www.kazu.com@gmail.com> - 0.3.9-1
- I didn't check http error for health check
- It isn't right file exist check method.
- Systemd has to restart on failure.
* Fri Jan 8 2021 pyama86 <www.kazu.com@gmail.com> - 0.3.8-1
- Force delete sockfile before starting server
* Mon Dec 21 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.7-1
- fix cache key
* Fri Dec 18 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.6-1
- improve ttl cache expiration.
* Tue Dec 1 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.5-1
- prefetch interval modify to half cache ttl
* Fri Oct 23 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.4-1
- Add AuthToken Authenticatin
- keep cache when error happened
* Fri Oct 8 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.3-1
- Set permissions (644) for systemd service file on CentOS.(#6)
* Mon Sep 7 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.2-1
- delete execflg systemd file
* Mon Aug 17 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.1-1
- change socket file permission 777
* Thu Aug 13 2020 pyama86 <www.kazu.com@gmail.com> - 0.3.0-1
- Add prefetch method
* Wed Aug 12 2020 pyama86 <www.kazu.com@gmail.com> - 0.2.0-1
- Change cache method to ttlcache
* Tue Jul 14 2020 pyama86 <www.kazu.com@gmail.com> - 0.1.0-1
- Initial packaging
