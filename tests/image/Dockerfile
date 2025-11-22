FROM golang:1.24.6-bookworm

# Install required packages
RUN apt-get update && apt-get install -y \
    iproute2 \
    tcpdump \
    iputils-ping \
    net-tools \
    iptables \
    curl \
    netcat-openbsd \
    iperf3 \
    vim \
    && rm -rf /var/lib/apt/lists/*

# Set up working directory
WORKDIR /workspace

# Copy the tcbroker source code
COPY . /workspace/

# Build tcbroker
RUN make build

# Copy entrypoint
COPY tests/image/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["sleep", "infinity"]
