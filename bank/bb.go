package bank

import (
	"errors"

	"github.com/PMoneda/flow"

	"bitbucket.org/mundipagg/boletoapi/config"
	"bitbucket.org/mundipagg/boletoapi/letters"
	"bitbucket.org/mundipagg/boletoapi/log"
	"bitbucket.org/mundipagg/boletoapi/models"
	"bitbucket.org/mundipagg/boletoapi/tmpl"
	"bitbucket.org/mundipagg/boletoapi/util"
)

type bankBB struct {
	validate *models.Validator
	log      *log.Log
}

//Cria uma nova instância do objeto que implementa os serviços do Banco do Brasil e configura os validadores que serão utilizados
func newBB() bankBB {
	b := bankBB{
		validate: models.NewValidator(),
		log:      log.CreateLog(),
	}
	b.validate.Push(bbValidateAccountAndDigit)
	b.validate.Push(bbValidateAgencyAndDigit)
	b.validate.Push(bbValidateOurNumber)
	b.validate.Push(bbValidateWalletVariation)
	b.validate.Push(baseValidateAmountInCents)
	b.validate.Push(baseValidateExpireDate)
	b.validate.Push(baseValidateBuyerDocumentNumber)
	b.validate.Push(baseValidateRecipientDocumentNumber)
	b.validate.Push(bbValidateTitleInstructions)
	b.validate.Push(bbValidateTitleDocumentNumber)
	return b
}

//Log retorna a referencia do log
func (b bankBB) Log() *log.Log {
	return b.log
}

func (b *bankBB) login(boleto *models.BoletoRequest) (string, error) {
	type token struct {
		AccessToken string `json:"access_token"`
	}
	body := "grant_type=client_credentials&scope=cobranca.registro-boletos"
	header := make(map[string]string)
	header["Content-Type"] = "application/x-www-form-urlencoded"
	header["Cache-Control"] = "no-cache"
	header["Authorization"] = "Basic " + util.Base64(boleto.Authentication.Username+":"+boleto.Authentication.Password)
	resp, _, err := util.Post(config.Get().URLBBToken, body, header)
	if err != nil {
		return "", err
	}
	tok := util.ParseJSON(resp, new(token)).(*token)
	return tok.AccessToken, errors.New("Saída inválida")
}

//ProcessBoleto faz o processamento de registro de boleto
func (b bankBB) ProcessBoleto(boleto *models.BoletoRequest) (models.BoletoResponse, error) {
	errs := b.ValidateBoleto(boleto)
	if len(errs) > 0 {
		return models.BoletoResponse{Errors: errs}, nil
	}
	tok, err := b.login(boleto)
	if err != nil {
		return models.BoletoResponse{}, err
	}
	boleto.Authentication.AuthorizationToken = tok
	return b.RegisterBoleto(boleto)
}

func (b bankBB) RegisterBoleto(boleto *models.BoletoRequest) (models.BoletoResponse, error) {
	r := flow.NewPipe()
	url := config.Get().URLBBRegisterBoleto
	from := letters.GetRegisterBoletoBBTmpl()
	r = r.From("message://?source=inline", boleto, from, tmpl.GetFuncMaps())
	r = r.To("logseq://?type=request&url="+url, b.log)
	r = r.To(url, map[string]string{"method": "POST", "insecureSkipVerify": "true"})
	r = r.To("logseq://?type=response&url="+url, b.log)
	ch := r.Choice()
	ch = ch.When(flow.Header("status").IsEqualTo("200"))
	ch = ch.To("transform://?format=xml", letters.GetBBregisterLetter(), letters.GetRegisterBoletoAPIResponseTmpl(), tmpl.GetFuncMaps())
	ch = ch.To("unmarshall://?format=json", new(models.BoletoResponse))
	ch = ch.Otherwise()
	ch = ch.To("logseq://?type=response&url="+url, b.log).To("apierro://")
	body := r.GetBody().(*models.BoletoResponse)
	return *body, nil
}

func (b bankBB) ValidateBoleto(boleto *models.BoletoRequest) models.Errors {
	return models.Errors(b.validate.Assert(boleto))
}

//GetBankNumber retorna o codigo do banco
func (b bankBB) GetBankNumber() models.BankNumber {
	return models.BancoDoBrasil
}
