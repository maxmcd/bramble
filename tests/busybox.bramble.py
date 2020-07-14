# busybox test
b = derivation(
    name="busybox_download",
    builder="fetch_url",
    environment={
        "decompress": False,
        "url": "$test_url/busybox.tar.gz",
        "hash": "ad60d52a6bb8300989499d1bfcdc0460de865f14ebe7791a00cb6279b7c88903",
    },
)

commands = """
[, [[, acpid, add-shell, addgroup, adduser, adjtimex, arch, arp, arping, ash, awk,
base64, basename, bc, beep, blkdiscard, blkid, blockdev, bootchartd, brctl, bunzip2,
bzcat, bzip2, cal, cat, chat, chattr, chgrp, chmod, chown, chpasswd, chpst, chroot,
chrt, chvt, cksum, clear, cmp, comm, conspy, cp, cpio, crond, crontab, cryptpw,
cttyhack, cut, date, dc, dd, deallocvt, delgroup, deluser, depmod, devmem, df,
dhcprelay, diff, dirname, dmesg, dnsd, dnsdomainname, dos2unix, dpkg, dpkg-deb, du,
dumpkmap, dumpleases, echo, ed, egrep, eject, env, envdir, envuidgid, ether-wake,
expand, expr, factor, fakeidentd, fallocate, false, fatattr, fbset, fbsplash, fdflush,
fdformat, fdisk, fgconsole, fgrep, find, findfs, flock, fold, free, freeramdisk, fsck,
fsck.minix, fsfreeze, fstrim, fsync, ftpd, ftpget, ftpput, fuser, getopt, getty, grep,
groups, gunzip, gzip, halt, hd, hdparm, head, hexdump, hexedit, hostid, hostname, httpd,
hush, hwclock, i2cdetect, i2cdump, i2cget, i2cset, i2ctransfer, id, ifconfig, ifdown,
ifenslave, ifplugd, ifup, inetd, init, insmod, install, ionice, iostat, ip, ipaddr,
ipcalc, ipcrm, ipcs, iplink, ipneigh, iproute, iprule, iptunnel, kbd_mode, kill,
killall, killall5, klogd, last, less, link, linux32, linux64, linuxrc, ln, loadfont,
loadkmap, logger, login, logname, logread, losetup, lpd, lpq, lpr, ls, lsattr, lsmod,
lsof, lspci, lsscsi, lsusb, lzcat, lzma, lzop, makedevs, makemime, man, md5sum, mdev,
mesg, microcom, mkdir, mkdosfs, mke2fs, mkfifo, mkfs.ext2, mkfs.minix, mkfs.vfat, mknod,
mkpasswd, mkswap, mktemp, modinfo, modprobe, more, mount, mountpoint, mpstat, mt, mv,
nameif, nanddump, nandwrite, nbd-client, nc, netstat, nice, nl, nmeter, nohup, nologin,
nproc, nsenter, nslookup, ntpd, nuke, od, openvt, partprobe, passwd, paste, patch,
pgrep, pidof, ping, ping6, pipe_progress, pivot_root, pkill, pmap, popmaildir, poweroff,
powertop, printenv, printf, ps, pscan, pstree, pwd, pwdx, raidautorun, rdate, rdev,
readahead, readlink, readprofile, realpath, reboot, reformime, remove-shell, renice,
reset, resize, resume, rev, rm, rmdir, rmmod, route, rpm, rpm2cpio, rtcwake, run-init,
run-parts, runlevel, runsv, runsvdir, rx, script, scriptreplay, sed, sendmail, seq,
setarch, setconsole, setfattr, setfont, setkeycodes, setlogcons, setpriv, setserial,
setsid, setuidgid, sh, sha1sum, sha256sum, sha3sum, sha512sum, showkey, shred, shuf,
slattach, sleep, smemcap, softlimit, sort, split, ssl_client, start-stop-daemon, stat,
strings, stty, su, sulogin, sum, sv, svc, svlogd, svok, swapoff, swapon, switch_root,
sync, sysctl, syslogd, tac, tail, tar, taskset, tc, tcpsvd, tee, telnet, telnetd, test,
tftp, tftpd, time, timeout, top, touch, tr, traceroute, traceroute6, true, truncate, ts,
tty, ttysize, tunctl, ubiattach, ubidetach, ubimkvol, ubirename, ubirmvol, ubirsvol,
ubiupdatevol, udhcpc, udhcpc6, udhcpd, udpsvd, uevent, umount, uname, unexpand, uniq,
unix2dos, unlink, unlzma, unshare, unxz, unzip, uptime, users, usleep, uudecode,
uuencode, vconfig, vi, vlock, volname, w, wall, watch, watchdog, wc, wget, which, who,
whoami, whois, xargs, xxd, xz, xzcat, yes, zcat, zcip
"""
command_list = " ".join(commands.strip().split("\n")).split(", ")
command_symlinks = "\n".join([("./ln -s busybox %s" % c) for c in command_list])

script = (
    """
set -x
$b/busybox-x86_64 cp $b/busybox-x86_64 $out/busybox
cd $out
./busybox ln -s busybox ln
%s
"""
    % command_symlinks
)


derivation(
    name="busybox",
    environment={"b": b},
    builder="%s/busybox-x86_64" % b,
    args=["sh", "-c", script],
)
