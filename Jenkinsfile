// Test on every branch. On main, auto-bump semver, tag, and publish.
// Feature branches only snapshot-build. Override with RELEASE_VERSION or RELEASE_BUMP.
//
// Releasing is goreleaser's job (see .goreleaser.yaml): it builds all five
// platform archives, writes checksums.txt, and creates the GitHub release.
// scripts/next-version.sh decides the tag when no override is set.

pipeline {
    agent {
        kubernetes {
            defaultContainer 'go'
            yaml '''
apiVersion: v1
kind: Pod
metadata:
  name: golang-pod
  labels:
    name: golang-pod
spec:
  serviceAccountName: jenkins
  containers:
    - name: 'go'
      image: 'golang:1.25'
      imagePullPolicy: Always
      tty: true
      securityContext:
        runAsUser: 0
      command:
        - sleep
      args:
        - 'infinity'
'''
        }
    }

    options {
        timeout(time: 30, unit: 'MINUTES')
        disableConcurrentBuilds()
        ansiColor('xterm')
    }

    parameters {
        string(
            name: 'RELEASE_VERSION',
            defaultValue: '',
            description: 'Optional exact tag to publish, for example v0.1.0. Leave empty to auto-bump on main.'
        )
        string(
            name: 'RELEASE_BUMP',
            defaultValue: '',
            description: 'Optional auto-bump: patch, minor, or major. Ignored when RELEASE_VERSION is set.'
        )
    }

    environment {
        HOME = "${env.WORKSPACE}"
        GOCACHE = "${env.WORKSPACE}/.cache/go-build"
        GOMODCACHE = "${env.WORKSPACE}/.cache/go-mod"
        GORELEASER_VERSION = '2.17.1'
    }

    stages {

        stage('Git') {
            steps {
                // Container runs as root; Jenkins owns the checkout. Git 2.35+
                // refuses that mix unless the workspace is a safe.directory.
                sh 'git config --global --add safe.directory "$WORKSPACE"'
            }
        }

        stage('Test') {
            steps {
                sh '''
                    go version
                    go vet ./...
                    test -z "$(git ls-files '*.go' | xargs gofmt -l)" || { echo "These files need gofmt:"; git ls-files '*.go' | xargs gofmt -l; exit 1; }
                    go test ./...
                '''
            }
        }

        stage('Install goreleaser') {
            steps {
                sh '''
                    curl -sfL "https://github.com/goreleaser/goreleaser/releases/download/v${GORELEASER_VERSION}/goreleaser_Linux_x86_64.tar.gz" \
                        | tar -xz -C /usr/local/bin goreleaser
                    goreleaser --version
                    goreleaser check
                '''
            }
        }

        // Proves the packaging config on branches without publishing.
        stage('Snapshot') {
            when {
                not { branch 'main' }
            }
            steps {
                sh 'goreleaser release --snapshot --clean --skip=publish'
                sh 'ls -lh dist/*.tar.gz dist/*.zip dist/checksums.txt'
            }
        }

        stage('Release') {
            when {
                branch 'main'
            }
            steps {
                script {
                    def override = params.RELEASE_VERSION?.trim()
                    if (override && !(override ==~ /^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$/)) {
                        error("RELEASE_VERSION must look like v1.2.3 or v1.2.3-rc1, got '${override}'")
                    }
                    def bump = params.RELEASE_BUMP?.trim()
                    if (bump && !(['patch', 'minor', 'major'].contains(bump))) {
                        error("RELEASE_BUMP must be patch, minor, or major, got '${bump}'")
                    }

                    withCredentials([string(credentialsId: 'threatoptic-cli-github-token', variable: 'GITHUB_TOKEN')]) {
                        withEnv(["RELEASE_TAG_OVERRIDE=${override ?: ''}", "RELEASE_BUMP=${bump ?: ''}"]) {
                            sh '''
                                set -e

                                # Multibranch checkouts are shallow and tagless; goreleaser needs
                                # the full history to build the changelog and next-version.sh
                                # needs existing tags.
                                git fetch --tags --unshallow || git fetch --tags

                                git config user.email "jenkins@threat-optic.com"
                                git config user.name "ThreatOptic Jenkins"
                                git remote set-url origin "https://x-access-token:${GITHUB_TOKEN}@github.com/ThreatOptic/CLI.git"

                                if [ -n "$RELEASE_TAG_OVERRIDE" ]; then
                                    RELEASE_TAG="$RELEASE_TAG_OVERRIDE"
                                else
                                    set +e
                                    if [ -n "$RELEASE_BUMP" ]; then
                                        RELEASE_TAG=$(bash scripts/next-version.sh "$RELEASE_BUMP")
                                    else
                                        RELEASE_TAG=$(bash scripts/next-version.sh)
                                    fi
                                    status=$?
                                    set -e
                                    if [ "$status" -eq 2 ]; then
                                        echo "HEAD is already released; nothing to publish."
                                        exit 0
                                    fi
                                    if [ "$status" -ne 0 ]; then
                                        exit "$status"
                                    fi
                                fi

                                if git rev-parse "$RELEASE_TAG" >/dev/null 2>&1; then
                                    echo "Tag $RELEASE_TAG already exists. Pick a new version."
                                    exit 1
                                fi

                                git tag -a "$RELEASE_TAG" -m "Release $RELEASE_TAG"
                                git push origin "$RELEASE_TAG"

                                goreleaser release --clean
                            '''
                        }
                    }
                }
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'dist/*.tar.gz,dist/*.zip,dist/checksums.txt', allowEmptyArchive: true, fingerprint: true
        }
    }
}
