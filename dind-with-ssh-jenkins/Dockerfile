#### credit to evarga/jenkins-slave and tehranian/dind-jenkins-slave
# This Dockerfile is used to build an image containing basic stuff to be used as a Jenkins slave build node.
FROM ubuntu:14.04
MAINTAINER Brandon Mangold <bmangold@docker.com>

# Make sure the package repository is up to date.

RUN apt-get update -qq && apt-get install -qqy \
    apt-transport-https \
    ca-certificates \
    curl \
    lxc \
    iptables
RUN apt-get -y upgrade

# Install a basic SSH server
RUN apt-get install -y openssh-server
RUN sed -i 's|session    required     pam_loginuid.so|session    optional     pam_loginuid.so|g' /etc/pam.d/sshd
RUN mkdir -p /var/run/sshd

# Install JDK 7 (latest edition)
RUN apt-get install -y openjdk-7-jdk

# Add user jenkins to the image
RUN adduser --quiet jenkins
# Set password for the jenkins user (you may want to alter this).
RUN echo "jenkins:jenkins" | chpasswd

# Standard SSH port
EXPOSE 22

#### tehranian/dind-jenkins-slave
# Install Docker from test Docker repositories.
RUN curl -sSL https://get.docker.com/ubuntu/ | sh

ADD wrapdocker /usr/local/bin/wrapdocker
RUN chmod +x /usr/local/bin/wrapdocker
VOLUME /var/lib/docker

# Make sure that the "jenkins" user is part of the "docker"
# group. Needed to access the docker daemon's unix socket.
RUN usermod -a -G docker jenkins
# Let Jenkins user do pretty much whatever it wants
RUN echo "jenkins ALL= NOPASSWD: ALL" >> /etc/sudoers
RUN mkdir -p /var/jenkins_home
RUN chown -R jenkins:jenkins /var/jenkins_home

# place the jenkins slave startup script into the container
ADD jenkins-slave-startup.sh /
RUN chmod +x /jenkins-slave-startup.sh
CMD ["/jenkins-slave-startup.sh"]
