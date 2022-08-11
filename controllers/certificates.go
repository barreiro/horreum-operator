package horreum

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	stdErrors "errors"
	"fmt"
	"math/big"
	"time"

	hyperfoilv1alpha1 "github.com/Hyperfoil/horreum-operator/api/v1alpha1"
	logr "github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func createCA(cr *hyperfoilv1alpha1.Horreum, r *HorreumReconciler, logger logr.Logger) (ca *x509.Certificate, caPrivKey *rsa.PrivateKey, err error) {
	caSecret := &corev1.Secret{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cr.Name + "-ca-certs", Namespace: cr.Namespace}, caSecret)
	var caPEMBytes []byte
	if err != nil && errors.IsNotFound(err) {
		caPrivKey, err = rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			logger.Error(err, "Cannot generate CA private key")
			return
		}
		caSecret.ObjectMeta = metav1.ObjectMeta{
			Name:      cr.Name + "-ca-certs",
			Namespace: cr.Namespace,
		}
		caSecret.Type = corev1.SecretTypeTLS
		caPrivateKeyPEM := new(bytes.Buffer)
		pem.Encode(caPrivateKeyPEM, &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
		})

		ca = &x509.Certificate{
			SerialNumber: big.NewInt(2019),
			Subject: pkix.Name{
				Organization: []string{"Horreum CA"},
				Country:      []string{"US"},
				Province:     []string{""},
				Locality:     []string{"Raleigh"},
				CommonName:   "ca.horreum.hyperfoil.io",
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().AddDate(10, 0, 0),
			IsCA:                  true,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
			BasicConstraintsValid: true,
		}

		var caBytes []byte
		caBytes, err = x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
		if err != nil {
			logger.Error(err, "Cannot generate CA certificate")
			return
		}
		caPEM := new(bytes.Buffer)
		pem.Encode(caPEM, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caBytes,
		})
		caPEMBytes = caPEM.Bytes()

		caSecret.Data = map[string][]byte{
			corev1.TLSPrivateKeyKey: caPrivateKeyPEM.Bytes(),
			corev1.TLSCertKey:       caPEM.Bytes(),
		}

		if err = controllerutil.SetControllerReference(cr, caSecret, r.Scheme); err != nil {
			return
		}

		logger.Info("Creating a new CA secret " + caSecret.GetName())
		if err = r.Create(context.TODO(), caSecret); err != nil {
			updateStatus(r, cr, "Error", "Cannot create CA private key secret")
			return
		}
	} else if err != nil {
		logger.Error(err, "Cannot fetch current CA certificates")
		return
	} else {
		caPEMBytes = caSecret.Data[corev1.TLSCertKey]
		caBlock, _ := pem.Decode(caPEMBytes)
		if caBlock == nil {
			err = stdErrors.New("cannot decode service-ca certificate (" + fmt.Sprint(len(caPEMBytes)) + " bytes)")
			return
		}
		ca, err = x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			logger.Error(err, "Cannot parse existing CA certificate")
			return
		}
		caPrivKeyPEM := caSecret.Data[corev1.TLSPrivateKeyKey]
		caPrivKeyBlock, _ := pem.Decode(caPrivKeyPEM)
		if caPrivKeyBlock == nil {
			err = stdErrors.New("cannot decode service-ca private key (" + fmt.Sprint(len(caPrivKeyPEM)) + " bytes)")
			return
		}
		caPrivKey, err = x509.ParsePKCS1PrivateKey(caPrivKeyBlock.Bytes)
		if err != nil {
			logger.Error(err, "Cannot parse existing CA private key")
			return
		}
	}

	serviceCaConfigMap := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: "service-ca.crt", Namespace: cr.Namespace}, serviceCaConfigMap)
	if err != nil && errors.IsNotFound(err) {
		serviceCaConfigMap.ObjectMeta = metav1.ObjectMeta{
			Namespace: cr.Namespace,
			Name:      "service-ca.crt",
		}
		serviceCaConfigMap.BinaryData = map[string][]byte{
			"service-ca.crt": caPEMBytes,
		}
		if err = controllerutil.SetControllerReference(cr, serviceCaConfigMap, r.Scheme); err != nil {
			return
		}
		logger.Info("Creating config map service-ca.crt with CA certificate")
		err = r.Create(context.TODO(), serviceCaConfigMap)
		if err != nil {
			logger.Error(err, "Cannot create/update config map with CA")
			return
		}
	} else if err != nil {
		logger.Error(err, "Cannot fetch current CA config map")
		return
	} else {
		logger.Info("CA config map is present, not doing anything")
	}
	return
}

func createServiceCert(cr *hyperfoilv1alpha1.Horreum, r *HorreumReconciler, logger logr.Logger,
	ca *x509.Certificate, caPrivKey *rsa.PrivateKey,
	resourceName string, serviceName string, serial int64) error {
	if ca == nil || caPrivKey == nil {
		return stdErrors.New("CA is nil")
	}
	certSecret := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: resourceName, Namespace: cr.Namespace}, certSecret)
	if err != nil && errors.IsNotFound(err) {

		sans := []string{
			serviceName + "." + cr.Namespace + ".svc",
			serviceName + "." + cr.Namespace + ".svc.cluster.local",
			"*." + serviceName + "." + cr.Namespace + ".svc",
			"*." + serviceName + "." + cr.Namespace + ".svc.cluster.local",
		}
		if cr.Spec.NodeHost != "" {
			sans = append(sans, cr.Spec.NodeHost)
		}
		cert := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject: pkix.Name{
				Organization: []string{"Horreum"},
				Country:      []string{"US"},
				Province:     []string{""},
				Locality:     []string{"Raleigh"},
				CommonName:   serviceName,
			},
			DNSNames:    sans,
			NotBefore:   time.Now(),
			NotAfter:    time.Now().AddDate(10, 0, 0),
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
			KeyUsage:    x509.KeyUsageDigitalSignature,
		}

		certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			logger.Error(err, "Cannot generate private key")
			return err
		}

		certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
		if err != nil {
			logger.Error(err, "Cannot generate a certificate")
			return err
		}

		certPEM := new(bytes.Buffer)
		pem.Encode(certPEM, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certBytes,
		})
		certPrivKeyPEM := new(bytes.Buffer)
		pem.Encode(certPrivKeyPEM, &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
		})

		certSecret.ObjectMeta = metav1.ObjectMeta{
			Namespace: cr.GetNamespace(),
			Name:      resourceName,
		}
		certSecret.Type = corev1.SecretTypeTLS
		certSecret.Data = map[string][]byte{
			corev1.TLSCertKey:       certPEM.Bytes(),
			corev1.TLSPrivateKeyKey: certPrivKeyPEM.Bytes(),
		}

		if err = controllerutil.SetControllerReference(cr, certSecret, r.Scheme); err != nil {
			return err
		}

		logger.Info("Creating new certificate " + certSecret.Name)
		err = r.Create(context.TODO(), certSecret)
		if err != nil {
			logger.Error(err, "Cannot create secret with service certificate")
		}
		return err
	} else if err == nil {
		logger.Info("Certificate " + resourceName + " is present, not doing anything")
		return nil
	} else {
		return err
	}
}
