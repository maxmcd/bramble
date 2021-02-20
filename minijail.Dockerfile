FROM debian

RUN apt-get update && apt-get install -y build-essential git libcap-dev

RUN git clone https://android.googlesource.com/platform/external/minijail
RUN cd minijail && make LIBDIR=/lib64
