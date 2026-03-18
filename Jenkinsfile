pipeline {
    agent {
        node {
            label 'buildkit'
        }
    }

    environment {
        REGISTRY = 'registry.adoah.dev'
        VERSION  = "${env.BRANCH_NAME}-${BUILD_NUMBER}"
    }

    post {
        always {
            cleanWs(deleteDirs: true, notFailBuild: false)
        }
    }

    stages {
        stage("Build") {
            failFast true
            parallel {
                stage("Build API") {
                    agent { label 'buildkit' }
                    environment {
                        IMAGE = 'projects/stasharr-api'
                    }
                    steps {
                        container(name: 'buildkitd') {
                            sh '''
                                buildctl \
                                    --addr tcp://buildkit.adoah.dev:1234 \
                                    --tlscert /root/.certs/tls.crt \
                                    --tlskey /root/.certs/tls.key \
                                    --tlscacert /root/.ca/ca.pem \
                                build \
                                    --frontend dockerfile.v0 \
                                    --local context=. \
                                    --local dockerfile=docker \
                                    --opt filename=api.Dockerfile \
                                    --opt target=production \
                                    --output type=image,name=${REGISTRY}/${IMAGE}:${VERSION},push=true
                            '''
                        }
                    }
                }
                stage("Build UI") {
                    agent { label 'buildkit' }
                    environment {
                        IMAGE = 'projects/stasharr-ui'
                    }
                    steps {
                        container(name: 'buildkitd') {
                            sh '''
                                buildctl \
                                    --addr tcp://buildkit.adoah.dev:1234 \
                                    --tlscert /root/.certs/tls.crt \
                                    --tlskey /root/.certs/tls.key \
                                    --tlscacert /root/.ca/ca.pem \
                                build \
                                    --frontend dockerfile.v0 \
                                    --local context=. \
                                    --local dockerfile=docker \
                                    --opt filename=ui.Dockerfile \
                                    --opt target=production \
                                    --output type=image,name=${REGISTRY}/${IMAGE}:${VERSION},push=true
                            '''
                        }
                    }
                }
            }
        }
        stage("Deploy") {
            agent { label 'helm-deploy' }
            when {
                branch 'master'
            }
            steps {
                container(name: 'helm') {
                    sh '''helm dependency build && helm upgrade --install stasharr .ci/chart \
                        --namespace nsfw \
                        --version ${BUILD_NUMBER} \
                        --set mononen-library-chart.global.imageDefaults.tag=${VERSION}
                    '''
                }
            }
        }
    }
}
