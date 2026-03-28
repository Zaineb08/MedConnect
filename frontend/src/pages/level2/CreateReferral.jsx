import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  BrainCircuit,
  Send,
  AlertCircle,
  Building2,
  Image as ImageIcon,
  X,
  Phone,
} from "lucide-react";
import {
  getDirectory,
  suggestDepartment,
  createReferral,
  uploadAttachments,
  queueReferralForSync,
  syncQueuedReferrals,
} from "../../services/api";

// Country codes for phone validation
const COUNTRY_CODES = [
  { code: "+212", name: "Maroc", flag: "🇲🇦" },
  { code: "+33", name: "France", flag: "🇫🇷" },
  { code: "+34", name: "Espagne", flag: "🇪🇸" },
  { code: "+49", name: "Allemagne", flag: "🇩🇪" },
  { code: "+39", name: "Italie", flag: "🇮🇹" },
  { code: "+44", name: "Royaume-Uni", flag: "🇬🇧" },
  { code: "+1", name: "USA/Canada", flag: "🇺🇸" },
  { code: "+213", name: "Algérie", flag: "🇩🇿" },
  { code: "+216", name: "Tunisie", flag: "🇹🇳" },
];

// Phone validation function
const validatePhoneNumber = (phone, countryCode) => {
  // Remove spaces and special characters
  const cleanPhone = phone.replace(/[\s\-\(\)]/g, "");

  // Check if phone starts with the country code prefix
  const codeWithoutPlus = countryCode.replace("+", "");

  // For Morocco (+212), validate local format
  if (countryCode === "+212") {
    // Can be: 06XXXXXXXX, 07XXXXXXXX, +2126XXXXXXXX, 2126XXXXXXXX
    const localMatch = cleanPhone.match(/^(0)?([67]\d{8})$/);
    const intlMatch = cleanPhone.match(/^(\+?212)?([67]\d{8})$/);

    if (localMatch || intlMatch) {
      return {
        valid: true,
        normalized: `+212${localMatch ? localMatch[2] : intlMatch[2]}`,
      };
    }
    return {
      valid: false,
      error: "Numéro marocain invalide. Format: 06XX XXXXXX",
    };
  }

  // For other countries, just check minimum length
  if (cleanPhone.length >= 8) {
    return { valid: true, normalized: `${countryCode}${cleanPhone}` };
  }

  return { valid: false, error: "Numéro de téléphone invalide" };
};

export default function CreateReferral() {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [departments, setDepartments] = useState([]);
  const [phoneError, setPhoneError] = useState("");
  const [selectedCountryCode, setSelectedCountryCode] = useState("+212");

  // Form State
  const [formData, setFormData] = useState({
    patient_cin: "",
    patient_name: "",
    patient_dob: "",
    patient_phone: "",
    patient_consent: false,
    department_id: "",
    symptoms: "",
    urgency: "MEDIUM",
    ai_suggested_dept: null, // Stores the exact AI string if accepted
  });

  // AI State
  const [aiLoading, setAiLoading] = useState(false);
  const [suggestion, setSuggestion] = useState(null);

  // Files State
  const [selectedFiles, setSelectedFiles] = useState([]);

  useEffect(() => {
    const fetchDepts = async () => {
      try {
        const data = await getDirectory();
        // Only allow referring to accepting departments
        setDepartments(data.departments.filter((d) => d.is_accepting));
      } catch (err) {
        setError("Impossible de charger la liste des départements.");
      }
    };
    fetchDepts();
  }, []);

  const handleChange = (e) => {
    const { name, value, type, checked } = e.target;

    // Clear phone error when user starts typing
    if (name === "patient_phone") {
      setPhoneError("");
    }

    setFormData({ ...formData, [name]: type === "checkbox" ? checked : value });
  };

  const handleCountryCodeChange = (e) => {
    setSelectedCountryCode(e.target.value);
    setPhoneError("");
  };

  const handlePhoneBlur = () => {
    if (!formData.patient_phone) return;

    const validation = validatePhoneNumber(
      formData.patient_phone,
      selectedCountryCode,
    );
    if (!validation.valid) {
      setPhoneError(validation.error);
    }
  };

  const handleAISuggest = async () => {
    if (
      !formData.symptoms ||
      formData.symptoms.length < 10 ||
      !formData.patient_dob
    ) {
      setError(
        "Veuillez saisir la date de naissance et au moins 10 caractères de symptômes pour l'IA.",
      );
      return;
    }

    setAiLoading(true);
    setError("");
    setSuggestion(null);

    try {
      const resp = await suggestDepartment({
        symptoms: formData.symptoms,
        patient_dob: formData.patient_dob,
      });
      setSuggestion(resp);
    } catch (err) {
      setError(
        "Erreur lors de l'analyse IA: " +
          (err.response?.data?.error || err.message),
      );
    } finally {
      setAiLoading(false);
    }
  };

  const acceptAiSuggestion = () => {
    if (!suggestion) return;

    // Find matching department ID from the name
    const match = departments.find(
      (d) =>
        d.name.toLowerCase() === suggestion.suggested_department.toLowerCase(),
    );

    if (match) {
      setFormData({
        ...formData,
        department_id: match.id,
        ai_suggested_dept: match.name,
        urgency: suggestion.urgency || "MEDIUM",
      });
    } else {
      setError(
        "Le CHU ne dispose pas de ce département ou il n'accepte pas de patients actuellement.",
      );
    }
  };

  const handleFileChange = (e) => {
    const files = Array.from(e.target.files);
    // Limit to 5 files
    if (selectedFiles.length + files.length > 5) {
      setError("Limite de 5 photos par référence.");
      return;
    }
    setSelectedFiles([...selectedFiles, ...files]);
  };

  const removeFile = (index) => {
    setSelectedFiles(selectedFiles.filter((_, i) => i !== index));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError("");

    // Validate phone before submission
    if (!formData.patient_phone) {
      setError("Le numéro de téléphone est requis");
      setLoading(false);
      return;
    }

    const phoneValidation = validatePhoneNumber(
      formData.patient_phone,
      selectedCountryCode,
    );
    if (!phoneValidation.valid) {
      setPhoneError(phoneValidation.error);
      setLoading(false);
      return;
    }

    if (!formData.patient_consent) {
      setError("Le consentement explicite du patient est obligatoire avant transmission.");
      setLoading(false);
      return;
    }

    try {
      const payload = {
        ...formData,
        patient_phone: phoneValidation.normalized,
      };

      if (!navigator.onLine) {
        if (selectedFiles.length > 0) {
          setError(
            "Mode hors ligne: envoyez la référence sans pièces jointes, puis ajoutez les fichiers une fois la connexion rétablie.",
          );
          setLoading(false);
          return;
        }

        const queueSize = await queueReferralForSync(payload);
        navigate("/dashboard", {
          state: {
            message: `Connexion indisponible. Référence chiffrée enregistrée localement (file d'attente: ${queueSize}). Elle sera transmise automatiquement dès le retour du réseau.`,
          },
        });
        return;
      }

      const resp = await createReferral(payload);

      // Upload attachments if any
      if (selectedFiles.length > 0) {
        await uploadAttachments(resp.referral_id, selectedFiles);
      }

      await syncQueuedReferrals();

      navigate("/dashboard", {
        state: { message: "Référence envoyée avec succès au CHU" },
      });
    } catch (err) {
      if (!err.response) {
        try {
          const queueSize = await queueReferralForSync({
            ...formData,
            patient_phone: phoneValidation.normalized,
          });
          navigate("/dashboard", {
            state: {
              message: `Erreur réseau détectée. Référence sauvegardée localement (file d'attente: ${queueSize}) et synchronisation automatique activée.`,
            },
          });
          return;
        } catch {
          setError(
            "Erreur réseau et échec de la sauvegarde locale. Veuillez réessayer.",
          );
          return;
        }
      }

      setError(
        err.response?.data?.error || "Erreur lors de l'envoi de la référence.",
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-3xl mx-auto pb-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">
          Nouvelle Référence CHU
        </h1>
        <p className="text-gray-500 text-sm mt-1">
          Dossier confidentiel. Les données personnelles seront chiffrées
          (AES-256).
        </p>
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-50 border-l-4 border-red-500 rounded-r-lg flex items-start">
          <AlertCircle className="w-5 h-5 text-red-500 mr-3 shrink-0 mt-0.5" />
          <p className="text-sm text-red-700">{error}</p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-8">
        {/* Patient Information Box */}
        <div className="bg-white rounded-2xl shadow-sm border border-gray-200 overflow-hidden">
          <div className="bg-gray-50 px-6 py-4 border-b border-gray-200">
            <h2 className="text-lg font-bold text-gray-900">
              Identité du Patient
            </h2>
          </div>

          <div className="p-6 grid grid-cols-1 md:grid-cols-2 gap-6">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">
                N° CIN / Passeport
              </label>
              <input
                type="text"
                name="patient_cin"
                required
                value={formData.patient_cin}
                onChange={handleChange}
                className="w-full border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500"
                placeholder="ex: F123456"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">
                Nom Complet (Identique CIN)
              </label>
              <input
                type="text"
                name="patient_name"
                required
                value={formData.patient_name}
                onChange={handleChange}
                className="w-full border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">
                Date de Naissance
              </label>
              <input
                type="date"
                name="patient_dob"
                required
                value={formData.patient_dob}
                onChange={handleChange}
                className="w-full border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">
                <Phone className="w-4 h-4 inline mr-1" />
                N° Téléphone (WhatsApp)
              </label>
              <div className="flex gap-2">
                <select
                  value={selectedCountryCode}
                  onChange={handleCountryCodeChange}
                  className="w-28 border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500 text-sm"
                >
                  {COUNTRY_CODES.map((country) => (
                    <option key={country.code} value={country.code}>
                      {country.flag} {country.code}
                    </option>
                  ))}
                </select>
                <input
                  type="tel"
                  name="patient_phone"
                  required
                  value={formData.patient_phone}
                  onChange={handleChange}
                  onBlur={handlePhoneBlur}
                  className={`flex-1 border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500 ${phoneError ? "border-red-500 focus:border-red-500 focus:ring-red-500" : ""}`}
                  placeholder={
                    selectedCountryCode === "+212"
                      ? "06 61 00 00 00"
                      : "6 12 34 56 78"
                  }
                />
              </div>
              {phoneError && (
                <p className="mt-1 text-sm text-red-600">{phoneError}</p>
              )}
              <p className="mt-1 text-xs text-gray-500">
                Format Maroc: 06XX XXXXXX | Format international: +212 6XX
                XXXXXX
              </p>
            </div>
          </div>
        </div>

        {/* Clinical Info Box */}
        <div className="bg-white rounded-2xl shadow-sm border border-gray-200 overflow-hidden">
          <div className="bg-brand-50 px-6 py-4 border-b border-brand-100 flex justify-between items-center flex-wrap gap-4">
            <h2 className="text-lg font-bold text-gray-900">
              Bilan Clinique & Triage
            </h2>

            <button
              type="button"
              onClick={handleAISuggest}
              disabled={aiLoading}
              className="inline-flex items-center px-4 py-2 bg-white border border-brand-300 rounded-lg text-sm font-semibold text-brand-700 hover:bg-brand-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-brand-500 transition-colors shadow-sm disabled:opacity-50"
            >
              {aiLoading ? (
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-brand-700 mr-2"></div>
              ) : (
                <BrainCircuit className="w-4 h-4 mr-2" />
              )}
              Aide au Triage (IA)
            </button>
          </div>

          {/* AI Suggestion Alert */}
          {suggestion && (
            <div className="bg-blue-50 p-4 border-b border-blue-100 flex flex-col md:flex-row md:items-center justify-between gap-4">
              <div>
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-sm font-bold text-blue-900">
                    Recommandation IA :
                  </span>
                  <span className="px-2 py-0.5 bg-blue-200 text-blue-800 text-xs font-bold rounded">
                    {suggestion.suggested_department}
                  </span>
                  <span className="text-xs text-blue-600 font-medium">
                    (Confiance: {Math.round(suggestion.confidence * 100)}%)
                  </span>
                  {suggestion.urgency && (
                    <span className="px-2 py-0.5 bg-indigo-200 text-indigo-800 text-xs font-bold rounded ml-2">
                      Urgence: {suggestion.urgency}
                    </span>
                  )}
                </div>
                <p className="text-sm text-blue-800 italic">
                  {suggestion.reasoning}
                </p>
              </div>
              <button
                type="button"
                onClick={acceptAiSuggestion}
                className="shrink-0 px-3 py-1.5 bg-blue-600 text-white text-sm font-medium rounded hover:bg-blue-700 transition-colors"
              >
                Appliquer
              </button>
            </div>
          )}

          <div className="p-6 space-y-6">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1.5">
                Symptômes & Motif (Détaillé)
              </label>
              <textarea
                name="symptoms"
                required
                rows={5}
                value={formData.symptoms}
                onChange={handleChange}
                className="w-full border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500"
                placeholder="Décrivez l'historique de la maladie, les signes cliniques, traitements en cours..."
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5 flex items-center gap-2">
                  <Building2 className="w-4 h-4 text-gray-400" />
                  Département CHU Visé
                </label>
                <select
                  name="department_id"
                  required
                  value={formData.department_id}
                  onChange={handleChange}
                  className={`w-full border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500 ${formData.ai_suggested_dept ? "bg-blue-50 border-blue-300" : ""}`}
                >
                  <option value="">Sélectionner un service...</option>
                  {departments.map((dept) => (
                    <option key={dept.id} value={dept.id}>
                      {dept.name}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">
                  Niveau d'Urgence
                </label>
                <select
                  name="urgency"
                  required
                  value={formData.urgency}
                  onChange={handleChange}
                  className="w-full border-gray-300 rounded-lg shadow-sm focus:border-brand-500 focus:ring-brand-500 font-medium"
                >
                  <option value="LOW" className="text-green-700">
                    Faible (LOW)
                  </option>
                  <option value="MEDIUM" className="text-yellow-700">
                    Moyen (MEDIUM)
                  </option>
                  <option value="HIGH" className="text-orange-700">
                    Élevé (HIGH)
                  </option>
                  <option value="CRITICAL" className="text-red-700">
                    Critique (CRITICAL)
                  </option>
                </select>
              </div>
            </div>

            {/* File Upload Section */}
            <div className="pt-6 border-t border-gray-100">
              <label className="block text-sm font-bold text-gray-700 mb-3 flex items-center gap-2">
                <ImageIcon className="w-5 h-5 text-gray-400" />
                Photos & Documents (Analyses, Radios, Photos cliniques)
              </label>

              <div className="flex flex-wrap gap-4">
                {selectedFiles.map((file, idx) => (
                  <div
                    key={idx}
                    className="relative w-24 h-24 bg-gray-50 rounded-xl border border-gray-200 flex items-center justify-center p-2 group overflow-hidden"
                  >
                    <div className="text-[10px] text-gray-500 font-bold break-all text-center">
                      {file.name.length > 20
                        ? file.name.substring(0, 15) + "..."
                        : file.name}
                    </div>
                    <button
                      type="button"
                      onClick={() => removeFile(idx)}
                      className="absolute top-1 right-1 bg-red-500 text-white rounded-full p-1 opacity-0 group-hover:opacity-100 transition-opacity"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </div>
                ))}

                {selectedFiles.length < 5 && (
                  <label className="w-24 h-24 border-2 border-dashed border-gray-300 rounded-xl flex flex-col items-center justify-center cursor-pointer hover:border-brand-500 hover:bg-brand-50 transition-all text-gray-400 hover:text-brand-600">
                    <ImageIcon className="w-8 h-8 mb-1" />
                    <span className="text-[10px] font-bold">Ajouter</span>
                    <input
                      type="file"
                      multiple
                      accept="image/*,application/pdf"
                      onChange={handleFileChange}
                      className="hidden"
                    />
                  </label>
                )}
              </div>
              <p className="text-[10px] text-gray-400 mt-3 font-medium uppercase tracking-wider">
                Formats acceptés: JPG, PNG, PDF • Max 5 fichiers
              </p>
            </div>

            <div className="pt-6 border-t border-gray-100">
              <label className="flex items-start gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  name="patient_consent"
                  checked={formData.patient_consent}
                  onChange={handleChange}
                  className="mt-1 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                />
                <span className="text-sm text-gray-700">
                  I confirm that I have obtained the patient's consent for processing and transferring this medical data.
                </span>
              </label>
              <p className="mt-2 text-xs text-gray-500">
                CNDP (Loi 09-08): this confirmation is mandatory to create a referral.
              </p>
            </div>
          </div>
        </div>

        <div className="pt-4 flex items-center justify-end gap-4 border-t border-gray-200">
          <button
            type="button"
            onClick={() => navigate("/dashboard")}
            className="px-6 py-3 text-sm font-semibold text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50 transition-colors"
          >
            Annuler
          </button>

          <button
            type="submit"
            disabled={loading}
            className="flex items-center px-8 py-3 bg-brand-600 text-white text-sm font-bold rounded-lg hover:bg-brand-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-brand-500 transition-colors shadow-md disabled:opacity-70"
          >
            {loading ? (
              <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-white mr-2"></div>
            ) : (
              <Send className="w-5 h-5 mr-2" />
            )}
            {loading ? "Chiffrement & Envoi..." : "Transmettre au CHU"}
          </button>
        </div>
      </form>
    </div>
  );
}
